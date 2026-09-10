import Foundation

/// Comprehensive LaTeX math and mathematical symbol processor.
/// Converts LaTeX formulas, inline/block math, arrows, operators, Greek letters,
/// superscripts, subscripts, and structural commands into clean, native Unicode typography.
public enum MathSymbolProcessor: Sendable {
    
    // MARK: - Symbol Dictionaries
    
    /// Pre-sorted symbol mappings (longest key first to prevent prefix shadowing)
    private static let symbolReplacements: [(pattern: String, replacement: String)] = [
        // Blackboard bold / Sets
        (#"\\mathbb\{R\}"#, "ℝ"), (#"\\mathbf\{R\}"#, "ℝ"),
        (#"\\mathbb\{N\}"#, "ℕ"), (#"\\mathbf\{N\}"#, "ℕ"),
        (#"\\mathbb\{Z\}"#, "ℤ"), (#"\\mathbf\{Z\}"#, "ℤ"),
        (#"\\mathbb\{Q\}"#, "ℚ"), (#"\\mathbf\{Q\}"#, "ℚ"),
        (#"\\mathbb\{C\}"#, "ℂ"), (#"\\mathbf\{C\}"#, "ℂ"),
        (#"\\mathbb\{E\}"#, "𝔼"), (#"\\mathbb\{P\}"#, "ℙ"),
        (#"\\mathbb\{H\}"#, "ℍ"), (#"\\mathbb\{F\}"#, "𝔽"),
        (#"\\mathcal\{L\}"#, "ℒ"), (#"\\mathcal\{O\}"#, "𝒪"),
        (#"\\mathcal\{N\}"#, "𝒩"), (#"\\mathcal\{H\}"#, "ℋ"),
        (#"\\mathcal\{F\}"#, "ℱ"), (#"\\mathcal\{D\}"#, "𝒟"),
        
        // Long arrows
        (#"\\longleftrightarrow"#, "⟷"),
        (#"\\Longleftrightarrow"#, "⟺"),
        (#"\\longrightarrow"#, "⟶"),
        (#"\\longleftarrow"#, "⟵"),
        (#"\\Longrightarrow"#, "⟹"),
        (#"\\Longleftarrow"#, "⟸"),
        (#"\\longmapsto"#, "⟼"),
        
        // Harpoons & standard arrows
        (#"\\rightleftharpoons"#, "⇌"),
        (#"\\rightharpoonup"#, "⇀"),
        (#"\\rightharpoondown"#, "⇁"),
        (#"\\leftharpoonup"#, "↼"),
        (#"\\leftharpoondown"#, "↽"),
        (#"\\hookrightarrow"#, "↪"),
        (#"\\hookleftarrow"#, "↩"),
        (#"\\leftrightarrow"#, "↔"),
        (#"\\Leftrightarrow"#, "⇔"),
        (#"\\rightarrow"#, "→"),
        (#"\\leftarrow"#, "←"),
        (#"\\Rightarrow"#, "⇒"),
        (#"\\Leftarrow"#, "⇐"),
        (#"\\updownarrow"#, "↕"),
        (#"\\Updownarrow"#, "⇕"),
        (#"\\uparrow"#, "↑"),
        (#"\\downarrow"#, "↓"),
        (#"\\Uparrow"#, "⇑"),
        (#"\\Downarrow"#, "⇓"),
        (#"\\nearrow"#, "↗"),
        (#"\\searrow"#, "↘"),
        (#"\\swarrow"#, "↙"),
        (#"\\nwarrow"#, "↖"),
        (#"\\mapsto"#, "↦"),
        (#"\\implies"#, "⇒"),
        (#"\\iff"#, "⇔"),
        (#"\\to\b"#, "→"),
        (#"\\gets\b"#, "←"),
        
        // Comparisons & Relations
        (#"\\leqslant"#, "≤"),
        (#"\\geqslant"#, "≥"),
        (#"\\subseteq"#, "⊆"),
        (#"\\supseteq"#, "⊇"),
        (#"\\subsetneq"#, "⊊"),
        (#"\\supsetneq"#, "⊋"),
        (#"\\nsubseteq"#, "⊈"),
        (#"\\nsupseteq"#, "⊉"),
        (#"\\parallel"#, "∥"),
        (#"\\nparallel"#, "∦"),
        (#"\\preceq"#, "⪯"),
        (#"\\succeq"#, "⪰"),
        (#"\\approx"#, "≈"),
        (#"\\simeq"#, "≃"),
        (#"\\cong"#, "≅"),
        (#"\\equiv"#, "≡"),
        (#"\\propto"#, "∝"),
        (#"\\asymp"#, "≍"),
        (#"\\doteq"#, "≐"),
        (#"\\prec"#, "≺"),
        (#"\\succ"#, "≻"),
        (#"\\sim"#, "∼"),
        (#"\\perp"#, "⊥"),
        (#"\\ll"#, "≪"),
        (#"\\gg"#, "≫"),
        (#"\\leq"#, "≤"),
        (#"\\geq"#, "≥"),
        (#"\\neq"#, "≠"),
        (#"\\le\b"#, "≤"),
        (#"\\ge\b"#, "≥"),
        (#"\\ne\b"#, "≠"),
        
        // Set & Logic
        (#"\\emptyset"#, "∅"),
        (#"\\empty\b"#, "∅"),
        (#"\\setminus"#, "∖"),
        (#"\\notin"#, "∉"),
        (#"\\subset"#, "⊂"),
        (#"\\supset"#, "⊃"),
        (#"\\forall"#, "∀"),
        (#"\\exists"#, "∃"),
        (#"\\nexists"#, "∄"),
        (#"\\lnot"#, "¬"),
        (#"\\land"#, "∧"),
        (#"\\wedge"#, "∧"),
        (#"\\lor"#, "∨"),
        (#"\\vee"#, "∨"),
        (#"\\top"#, "⊤"),
        (#"\\bot"#, "⊥"),
        (#"\\vdash"#, "⊢"),
        (#"\\vDash"#, "⊨"),
        (#"\\cup"#, "∪"),
        (#"\\cap"#, "∩"),
        (#"\\in\b"#, "∈"),
        (#"\\ni\b"#, "∋"),
        (#"\\owns"#, "∋"),
        (#"\\neg"#, "¬"),
        
        // Arithmetic & Operators
        (#"\\times"#, "×"),
        (#"\\div"#, "÷"),
        (#"\\pm"#, "±"),
        (#"\\mp"#, "∓"),
        (#"\\cdot"#, "·"),
        (#"\\cdots"#, "⋯"),
        (#"\\ldots"#, "…"),
        (#"\\ddots"#, "⋱"),
        (#"\\vdots"#, "⋮"),
        (#"\\bullet"#, "•"),
        (#"\\circ"#, "∘"),
        (#"\\star"#, "⋆"),
        (#"\\ast"#, "∗"),
        (#"\\oplus"#, "⊕"),
        (#"\\ominus"#, "⊖"),
        (#"\\otimes"#, "⊗"),
        (#"\\oslash"#, "⊘"),
        (#"\\odot"#, "⊙"),
        (#"\\dagger"#, "†"),
        (#"\\ddagger"#, "‡"),
        
        // Calculus & Analysis
        (#"\\iiint"#, "∭"),
        (#"\\iint"#, "∬"),
        (#"\\oint"#, "∮"),
        (#"\\int"#, "∫"),
        (#"\\sum"#, "∑"),
        (#"\\prod"#, "∏"),
        (#"\\coprod"#, "∐"),
        (#"\\partial"#, "∂"),
        (#"\\nabla"#, "∇"),
        (#"\\infty"#, "∞"),
        (#"\\sqrt"#, "√"),
        (#"\\angle"#, "∠"),
        (#"\\triangle"#, "△"),
        (#"\\diamond"#, "◇"),
        (#"\\square"#, "□"),
        (#"\\degree"#, "°"),
        (#"\\hbar"#, "ℏ"),
        (#"\\ell"#, "ℓ"),
        (#"\\aleph"#, "ℵ"),
        (#"\\Re"#, "ℜ"),
        (#"\\Im"#, "ℑ"),
        
        // Greek Uppercase
        (#"\\Gamma"#, "Γ"),
        (#"\\Delta"#, "Δ"),
        (#"\\Theta"#, "Θ"),
        (#"\\Lambda"#, "Λ"),
        (#"\\Xi"#, "Ξ"),
        (#"\\Pi"#, "Π"),
        (#"\\Sigma"#, "Σ"),
        (#"\\Upsilon"#, "Υ"),
        (#"\\Phi"#, "Φ"),
        (#"\\Psi"#, "Ψ"),
        (#"\\Omega"#, "Ω"),
        
        // Greek Lowercase
        (#"\\varepsilon"#, "ε"),
        (#"\\vartheta"#, "ϑ"),
        (#"\\varrho"#, "ϱ"),
        (#"\\varsigma"#, "ς"),
        (#"\\varphi"#, "φ"),
        (#"\\varpi"#, "ϖ"),
        (#"\\alpha"#, "α"),
        (#"\\beta"#, "β"),
        (#"\\gamma"#, "γ"),
        (#"\\delta"#, "δ"),
        (#"\\epsilon"#, "ϵ"),
        (#"\\zeta"#, "ζ"),
        (#"\\eta"#, "η"),
        (#"\\theta"#, "θ"),
        (#"\\iota"#, "ι"),
        (#"\\kappa"#, "κ"),
        (#"\\lambda"#, "λ"),
        (#"\\mu"#, "μ"),
        (#"\\nu"#, "ν"),
        (#"\\xi"#, "ξ"),
        (#"\\pi"#, "π"),
        (#"\\rho"#, "ρ"),
        (#"\\sigma"#, "σ"),
        (#"\\tau"#, "τ"),
        (#"\\upsilon"#, "υ"),
        (#"\\phi"#, "ϕ"),
        (#"\\chi"#, "χ"),
        (#"\\psi"#, "ψ"),
        (#"\\omega"#, "ω")
    ]
    
    // MARK: - Superscripts and Subscripts Maps
    
    private static let superscriptDict: [Character: Character] = [
        "0": "⁰", "1": "¹", "2": "²", "3": "³", "4": "⁴",
        "5": "⁵", "6": "⁶", "7": "⁷", "8": "⁸", "9": "⁹",
        "+": "⁺", "-": "⁻", "=": "⁼", "(": "⁽", ")": "⁾",
        "n": "ⁿ", "i": "ⁱ", "j": "ʲ", "a": "ᵃ", "b": "ᵇ",
        "c": "ᶜ", "d": "ᵈ", "e": "ᵉ", "f": "ᶠ", "g": "ᵍ",
        "h": "ʰ", "k": "ᵏ", "l": "ˡ", "m": "ᵐ", "o": "ᵒ",
        "p": "ᵖ", "r": "ʳ", "s": "ˢ", "t": "ᵗ", "u": "ᵘ",
        "v": "ᵛ", "w": "ʷ", "x": "ˣ", "y": "ʸ", "z": "ᶻ",
        "T": "ᵀ", "*": "*"
    ]
    
    private static let subscriptDict: [Character: Character] = [
        "0": "₀", "1": "₁", "2": "₂", "3": "₃", "4": "₄",
        "5": "₅", "6": "₆", "7": "₇", "8": "₈", "9": "₉",
        "+": "₊", "-": "₋", "=": "₌", "(": "₍", ")": "₎",
        "a": "ₐ", "e": "ₑ", "h": "ₕ", "i": "ᵢ", "j": "ⱼ",
        "k": "ₖ", "l": "ₗ", "m": "ₘ", "n": "ₙ", "o": "ₒ",
        "p": "ₚ", "r": "ᵣ", "s": "ₛ", "t": "ₜ", "u": "ᵤ",
        "v": "ᵥ", "x": "ₓ"
    ]
    
    // MARK: - Precompiled Regular Expressions & Cache
    
    private final class ProcessedMathCache: @unchecked Sendable {
        static let shared = ProcessedMathCache()
        private let lock = NSLock()
        private var cache: [Int: String] = [:]
        
        func get(_ hash: Int) -> String? {
            lock.lock()
            defer { lock.unlock() }
            return cache[hash]
        }
        
        func set(_ hash: Int, value: String) {
            lock.lock()
            defer { lock.unlock() }
            if cache.count > 500 {
                cache.removeAll(keepingCapacity: true)
            }
            cache[hash] = value
        }
    }
    
    private static let precompiledSymbolReplacements: [(regex: NSRegularExpression, replacement: String)] = {
        symbolReplacements.compactMap { (pattern, replacement) in
            if let reg = try? NSRegularExpression(pattern: pattern) {
                return (reg, replacement)
            }
            return nil
        }
    }()
    
    private static let blockRegex = try? NSRegularExpression(pattern: #"```[a-zA-Z0-9_\-]*\n[\s\S]*?```"#)
    private static let inlineRegex = try? NSRegularExpression(pattern: #"`[^`\n]+`"#)
    private static let displayBlockRegex = try? NSRegularExpression(pattern: #"\$\$(.*?)\$\$|\\\[(.*?)\\\]"#, options: [.dotMatchesLineSeparators])
    private static let inlineMathRegex = try? NSRegularExpression(pattern: #"(?<!\\)\$(?!\s)([^$\n]+?)(?<!\s)(?<!\\)\$|\\\((.*?)\\\)"#)
    private static let textWrapperRegex = try? NSRegularExpression(pattern: #"\\(?:text|mathrm|mathbf|mathit|operatorname|pmb)\{([^}]*)\}"#)
    private static let fracRegex = try? NSRegularExpression(pattern: #"\\frac\{([^}]*)\}\{([^}]*)\}"#)
    private static let sqrtNRegex = try? NSRegularExpression(pattern: #"\\sqrt\[([^\]]*)\]\{([^}]*)\}"#)
    private static let sqrtRegex = try? NSRegularExpression(pattern: #"\\sqrt\{([^}]*)\}"#)
    private static let supGroupRegex = try? NSRegularExpression(pattern: #"\^\{([0-9a-zA-Z\+\-\=\(\)\*]+)\}"#)
    private static let supSingleRegex = try? NSRegularExpression(pattern: #"\^([0-9a-zA-Z\+\-\*])"#)
    private static let subGroupRegex = try? NSRegularExpression(pattern: #"_\{([0-9a-zA-Z\+\-\=\(\)]+)\}"#)
    private static let subSingleRegex = try? NSRegularExpression(pattern: #"_([0-9a-zA-Z])"#)
    
    // MARK: - Main Processing Entry Point
    
    /// Processes Markdown text to replace all LaTeX math symbols with native Unicode characters,
    /// safely preserving fenced code blocks and inline code spans.
    public static func process(_ text: String) -> String {
        guard !text.isEmpty else { return text }
        // Fast-path: If text does not contain backslash or dollar sign, no LaTeX math can be present
        guard text.contains("\\") || text.contains("$") else { return text }
        
        let hash = text.hashValue
        if let cached = ProcessedMathCache.shared.get(hash) {
            return cached
        }
        
        var protectedText = text
        
        // 1. Protect fenced code blocks ```...```
        var codeBlocks: [String] = []
        if let blockRegex = Self.blockRegex {
            let nsText = protectedText as NSString
            let matches = blockRegex.matches(in: protectedText, range: NSRange(location: 0, length: nsText.length))
            for match in matches.reversed() {
                let block = nsText.substring(with: match.range)
                let token = "XXAGYBLOCKTOKEN\(codeBlocks.count)XX"
                codeBlocks.append(block)
                protectedText = (protectedText as NSString).replacingCharacters(in: match.range, with: token)
            }
        }
        
        // 2. Protect inline code spans `code`
        var inlineCodeSpans: [String] = []
        if let inlineRegex = Self.inlineRegex {
            let nsText = protectedText as NSString
            let matches = inlineRegex.matches(in: protectedText, range: NSRange(location: 0, length: nsText.length))
            for match in matches.reversed() {
                let codeSpan = nsText.substring(with: match.range)
                let token = "XXAGYINLINETOKEN\(inlineCodeSpans.count)XX"
                inlineCodeSpans.append(codeSpan)
                protectedText = (protectedText as NSString).replacingCharacters(in: match.range, with: token)
            }
        }
        
        // 3. Process display math blocks: $$...$$ and \[...\]
        if let displayBlockRegex = Self.displayBlockRegex {
            protectedText = replaceRegexMatches(in: protectedText, regex: displayBlockRegex) { matchText in
                let content = matchText
                    .trimmingCharacters(in: CharacterSet(charactersIn: "$"))
                    .trimmingCharacters(in: .whitespacesAndNewlines)
                    .replacingOccurrences(of: #"\\["#, with: "")
                    .replacingOccurrences(of: #"\]"#, with: "")
                return cleanMathExpression(content)
            }
        }
        
        // 4. Process inline math: $...$ and \(...\)
        // Guard against accidental currency matches ($100 and $200) by ensuring no leading/trailing spaces
        if let inlineMathRegex = Self.inlineMathRegex {
            protectedText = replaceRegexMatches(in: protectedText, regex: inlineMathRegex) { matchText in
                var inner = matchText
                if inner.hasPrefix("$") && inner.hasSuffix("$") && inner.count >= 2 {
                    inner = String(inner.dropFirst().dropLast())
                } else if inner.hasPrefix(#"\("#) && inner.hasSuffix(#"\)"#) && inner.count >= 4 {
                    inner = String(inner.dropFirst(2).dropLast(2))
                }
                return cleanMathExpression(inner)
            }
        }
        
        // 5. Transform ONLY standalone LaTeX symbol commands in prose (e.g. \rightarrow outside of $)
        // CRITICAL: NEVER run subscripts (_), superscripts (^), fractions, or bracket cleanups on bare prose!
        if protectedText.contains("\\") {
            for (reg, repl) in precompiledSymbolReplacements {
                protectedText = reg.stringByReplacingMatches(
                    in: protectedText,
                    range: NSRange(location: 0, length: (protectedText as NSString).length),
                    withTemplate: repl
                )
            }
        }
        
        // 6. Restore inline code spans
        for (idx, span) in inlineCodeSpans.enumerated() {
            let token = "XXAGYINLINETOKEN\(idx)XX"
            protectedText = protectedText.replacingOccurrences(of: token, with: span)
        }
        
        // 7. Restore fenced code blocks
        for (idx, block) in codeBlocks.enumerated() {
            let token = "XXAGYBLOCKTOKEN\(idx)XX"
            protectedText = protectedText.replacingOccurrences(of: token, with: block)
        }
        
        ProcessedMathCache.shared.set(hash, value: protectedText)
        return protectedText
    }
    
    // MARK: - Math Expression Cleaner
    
    /// Cleans a math expression, stripping wrappers and translating LaTeX symbols
    public static func cleanMathExpression(_ input: String) -> String {
        var str = input
        
        // 1. Text wrappers: \text{...}, \mathrm{...}, \mathbf{...}, etc.
        if let textWrapperRegex = Self.textWrapperRegex {
            str = textWrapperRegex.stringByReplacingMatches(in: str, range: NSRange(location: 0, length: (str as NSString).length), withTemplate: "$1")
        }
        
        // 2. Fractions: \frac{a}{b} -> a / b
        if let fracRegex = Self.fracRegex {
            str = fracRegex.stringByReplacingMatches(in: str, range: NSRange(location: 0, length: (str as NSString).length), withTemplate: "$1 / $2")
        }
        
        // 3. Square roots: \sqrt{x} -> √(x), \sqrt[n]{x} -> ⁿ√(x)
        if let sqrtNRegex = Self.sqrtNRegex {
            str = sqrtNRegex.stringByReplacingMatches(in: str, range: NSRange(location: 0, length: (str as NSString).length), withTemplate: "$1√($2)")
        }
        if let sqrtRegex = Self.sqrtRegex {
            str = sqrtRegex.stringByReplacingMatches(in: str, range: NSRange(location: 0, length: (str as NSString).length), withTemplate: "√($1)")
        }
        
        // 4. Bracket cleanups
        str = str
            .replacingOccurrences(of: #"\left("#, with: "(")
            .replacingOccurrences(of: #"\right)"#, with: ")")
            .replacingOccurrences(of: #"\left["#, with: "[")
            .replacingOccurrences(of: #"\right]"#, with: "]")
            .replacingOccurrences(of: #"\left\{"#, with: "{")
            .replacingOccurrences(of: #"\right\}"#, with: "}")
            .replacingOccurrences(of: #"\{"#, with: "{")
            .replacingOccurrences(of: #"\}"#, with: "}")
            .replacingOccurrences(of: #"\%"#, with: "%")
            .replacingOccurrences(of: #"\_"#, with: "_")
            .replacingOccurrences(of: #"\&"#, with: "&")
            .replacingOccurrences(of: #"\,"#, with: " ")
            .replacingOccurrences(of: #"\;"#, with: " ")
            .replacingOccurrences(of: #"\quad"#, with: " ")
            .replacingOccurrences(of: #"\qquad"#, with: "  ")
        
        // 5. Replace LaTeX symbols (arrows, relations, operators, Greek)
        if str.contains("\\") {
            for (reg, repl) in precompiledSymbolReplacements {
                str = reg.stringByReplacingMatches(in: str, range: NSRange(location: 0, length: (str as NSString).length), withTemplate: repl)
            }
        }
        
        // 6. Convert superscripts: ^{...} and ^x
        if str.contains("^") {
            if let supGroupRegex = Self.supGroupRegex {
                let nsStr = str as NSString
                let matches = supGroupRegex.matches(in: str, range: NSRange(location: 0, length: nsStr.length))
                for match in matches.reversed() {
                    let inner = nsStr.substring(with: match.range(at: 1))
                    let converted = String(inner.map { superscriptDict[$0] ?? $0 })
                    str = (str as NSString).replacingCharacters(in: match.range, with: converted)
                }
            }
            if let supSingleRegex = Self.supSingleRegex {
                let nsStr = str as NSString
                let matches = supSingleRegex.matches(in: str, range: NSRange(location: 0, length: nsStr.length))
                for match in matches.reversed() {
                    let inner = nsStr.substring(with: match.range(at: 1))
                    if let firstChar = inner.first, let sup = superscriptDict[firstChar] {
                        str = (str as NSString).replacingCharacters(in: match.range, with: String(sup))
                    }
                }
            }
        }
        
        // 7. Convert subscripts: _{...} and _x
        if str.contains("_") {
            if let subGroupRegex = Self.subGroupRegex {
                let nsStr = str as NSString
                let matches = subGroupRegex.matches(in: str, range: NSRange(location: 0, length: nsStr.length))
                for match in matches.reversed() {
                    let inner = nsStr.substring(with: match.range(at: 1))
                    let converted = String(inner.map { subscriptDict[$0] ?? $0 })
                    str = (str as NSString).replacingCharacters(in: match.range, with: converted)
                }
            }
            if let subSingleRegex = Self.subSingleRegex {
                let nsStr = str as NSString
                let matches = subSingleRegex.matches(in: str, range: NSRange(location: 0, length: nsStr.length))
                for match in matches.reversed() {
                    let inner = nsStr.substring(with: match.range(at: 1))
                    if let firstChar = inner.first, let sub = subscriptDict[firstChar] {
                        str = (str as NSString).replacingCharacters(in: match.range, with: String(sub))
                    }
                }
            }
        }
        
        return str.trimmingCharacters(in: .whitespaces)
    }
    
    // MARK: - Regex Helper
    
    private static func replaceRegexMatches(in text: String, regex: NSRegularExpression, transform: (String) -> String) -> String {
        let nsText = text as NSString
        let matches = regex.matches(in: text, range: NSRange(location: 0, length: nsText.length))
        guard !matches.isEmpty else { return text }
        
        var result = text
        for match in matches.reversed() {
            let matchText = nsText.substring(with: match.range)
            let transformed = transform(matchText)
            result = (result as NSString).replacingCharacters(in: match.range, with: transformed)
        }
        return result
    }
}
