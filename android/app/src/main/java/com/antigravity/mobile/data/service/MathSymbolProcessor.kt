package com.antigravity.mobile.data.service

/**
 * 1:1 Kotlin port of iOS MathSymbolProcessor.swift.
 * Comprehensive LaTeX math and mathematical symbol processor.
 * Converts LaTeX formulas, inline/block math, arrows, operators, Greek letters,
 * superscripts, subscripts, and structural commands into clean, native Unicode typography.
 */
object MathSymbolProcessor {

    private val superscriptMap = mapOf(
        '0' to '⁰', '1' to '¹', '2' to '²', '3' to '³', '4' to '⁴',
        '5' to '⁵', '6' to '⁶', '7' to '⁷', '8' to '⁸', '9' to '⁹',
        '+' to '⁺', '-' to '⁻', '=' to '⁼', '(' to '⁽', ')' to '⁾',
        'n' to 'ⁿ', 'i' to 'ⁱ', 'x' to 'ˣ', 'y' to 'ʸ', 'a' to 'ᵃ',
        'b' to 'ᵇ', 'c' to 'ᶜ', 'd' to 'ᵈ', 'e' to 'ᵉ', 'f' to 'ᶠ',
        'k' to 'ᵏ', 'm' to 'ᵐ', 'p' to 'ᵖ', 'r' to 'ʳ', 't' to 'ᵗ'
    )

    private val subscriptMap = mapOf(
        '0' to '₀', '1' to '₁', '2' to '₂', '3' to '₃', '4' to '₄',
        '5' to '₅', '6' to '₆', '7' to '₇', '8' to '₈', '9' to '₉',
        '+' to '₊', '-' to '₋', '=' to '₌', '(' to '₍', ')' to '₎',
        'a' to 'ₐ', 'e' to 'ₑ', 'h' to 'ₕ', 'i' to 'ᵢ', 'j' to 'ⱼ',
        'k' to 'ₖ', 'l' to 'ₗ', 'm' to 'ₘ', 'n' to 'ₙ', 'o' to 'ₒ',
        'p' to 'ₚ', 'r' to 'ᵣ', 's' to 'ₛ', 't' to 'ₜ', 'u' to 'ᵤ',
        'v' to 'ᵥ', 'x' to 'ₓ'
    )

    // Symbol dictionary sorted longest first
    private val symbols = listOf(
        // Blackboard bold
        "\\mathbb{R}" to "ℝ", "\\mathbf{R}" to "ℝ",
        "\\mathbb{N}" to "ℕ", "\\mathbf{N}" to "ℕ",
        "\\mathbb{Z}" to "ℤ", "\\mathbf{Z}" to "ℤ",
        "\\mathbb{Q}" to "ℚ", "\\mathbf{Q}" to "ℚ",
        "\\mathbb{C}" to "ℂ", "\\mathbf{C}" to "ℂ",
        "\\mathbb{E}" to "𝔼", "\\mathbb{P}" to "ℙ",
        "\\mathbb{H}" to "ℍ", "\\mathbb{F}" to "𝔽",
        "\\mathcal{L}" to "ℒ", "\\mathcal{O}" to "𝒪",
        "\\mathcal{N}" to "𝒩", "\\mathcal{H}" to "ℋ",
        "\\mathcal{F}" to "ℱ", "\\mathcal{D}" to "𝒟",

        // Long arrows
        "\\longleftrightarrow" to "⟷",
        "\\Longleftrightarrow" to "⟺",
        "\\longrightarrow" to "⟶",
        "\\longleftarrow" to "⟵",
        "\\Longrightarrow" to "⟹",
        "\\Longleftarrow" to "⟸",
        "\\longmapsto" to "⟼",

        // Harpoons & standard arrows
        "\\rightleftharpoons" to "⇌",
        "\\rightharpoonup" to "⇀",
        "\\rightharpoondown" to "⇁",
        "\\leftharpoonup" to "↼",
        "\\leftharpoondown" to "↽",
        "\\hookrightarrow" to "↪",
        "\\hookleftarrow" to "↩",
        "\\leftrightarrow" to "↔",
        "\\Leftrightarrow" to "⇔",
        "\\rightarrow" to "→",
        "\\leftarrow" to "←",
        "\\Rightarrow" to "⇒",
        "\\Leftarrow" to "⇐",
        "\\updownarrow" to "↕",
        "\\Updownarrow" to "⇕",
        "\\uparrow" to "↑",
        "\\downarrow" to "↓",
        "\\Uparrow" to "⇑",
        "\\Downarrow" to "⇓",
        "\\nearrow" to "↗",
        "\\searrow" to "↘",
        "\\swarrow" to "↙",
        "\\nwarrow" to "↖",
        "\\mapsto" to "↦",
        "\\implies" to "⇒",
        "\\iff" to "⇔",
        "\\to" to "→",
        "\\gets" to "←",

        // Comparisons & Relations
        "\\leqslant" to "≤",
        "\\geqslant" to "≥",
        "\\subseteq" to "⊆",
        "\\supseteq" to "⊇",
        "\\subsetneq" to "⊊",
        "\\supsetneq" to "⊋",
        "\\nsubseteq" to "⊈",
        "\\nsupseteq" to "⊉",
        "\\parallel" to "∥",
        "\\nparallel" to "∦",
        "\\preceq" to "⪯",
        "\\succeq" to "⪰",
        "\\approx" to "≈",
        "\\simeq" to "≃",
        "\\cong" to "≅",
        "\\equiv" to "≡",
        "\\propto" to "∝",
        "\\asymp" to "≍",
        "\\doteq" to "≐",
        "\\prec" to "≺",
        "\\succ" to "≻",
        "\\sim" to "∼",
        "\\perp" to "⊥",
        "\\ll" to "≪",
        "\\gg" to "≫",
        "\\leq" to "≤",
        "\\geq" to "≥",
        "\\neq" to "≠",
        "\\le" to "≤",
        "\\ge" to "≥",
        "\\ne" to "≠",

        // Set & Logic
        "\\emptyset" to "∅",
        "\\empty" to "∅",
        "\\setminus" to "∖",
        "\\notin" to "∉",
        "\\forall" to "∀",
        "\\exists" to "∃",
        "\\nexists" to "∄",
        "\\cup" to "∪",
        "\\cap" to "∩",
        "\\in" to "∈",
        "\\ni" to "∋",
        "\\land" to "∧",
        "\\lor" to "∨",
        "\\neg" to "¬",
        "\\top" to "⊤",
        "\\bot" to "⊥",
        "\\models" to "⊨",
        "\\vdash" to "⊢",

        // Operators & Operations
        "\\otimes" to "⊗",
        "\\oplus" to "⊕",
        "\\odot" to "⊙",
        "\\boxtimes" to "⊠",
        "\\boxplus" to "⊞",
        "\\circ" to "∘",
        "\\bullet" to "•",
        "\\times" to "×",
        "\\div" to "÷",
        "\\pm" to "±",
        "\\mp" to "∓",
        "\\ast" to "∗",
        "\\star" to "⋆",
        "\\cdot" to "·",
        "\\cdots" to "⋯",
        "\\ldots" to "…",
        "\\ddots" to "⋱",
        "\\vdots" to "⋮",
        "\\diamond" to "◇",
        "\\triangle" to "△",
        "\\nabla" to "∇",
        "\\partial" to "∂",
        "\\infty" to "∞",
        "\\aleph" to "ℵ",
        "\\hbar" to "ℏ",
        "\\ell" to "ℓ",
        "\\Re" to "ℜ",
        "\\Im" to "ℑ",
        "\\wp" to "℘",

        // Calculus
        "\\iint" to "∬",
        "\\iiint" to "∭",
        "\\oint" to "∮",
        "\\int" to "∫",
        "\\sum" to "∑",
        "\\prod" to "∏",
        "\\coprod" to "∐",

        // Greek uppercase
        "\\Gamma" to "Γ", "\\Delta" to "Δ", "\\Theta" to "Θ",
        "\\Lambda" to "Λ", "\\Xi" to "Ξ", "\\Pi" to "Π",
        "\\Sigma" to "Σ", "\\Upsilon" to "Υ", "\\Phi" to "Φ",
        "\\Psi" to "Ψ", "\\Omega" to "Ω",

        // Greek lowercase
        "\\varepsilon" to "ε", "\\vartheta" to "ϑ", "\\varpi" to "ϖ",
        "\\varrho" to "ϱ", "\\varsigma" to "ς", "\\varphi" to "ϕ",
        "\\alpha" to "α", "\\beta" to "β", "\\gamma" to "γ",
        "\\delta" to "δ", "\\epsilon" to "ϵ", "\\zeta" to "ζ",
        "\\eta" to "η", "\\theta" to "θ", "\\iota" to "ι",
        "\\kappa" to "κ", "\\lambda" to "λ", "\\mu" to "μ",
        "\\nu" to "ν", "\\xi" to "ξ", "\\omicron" to "ο",
        "\\pi" to "π", "\\rho" to "ρ", "\\sigma" to "σ",
        "\\tau" to "τ", "\\upsilon" to "υ", "\\phi" to "φ",
        "\\chi" to "χ", "\\psi" to "ψ", "\\omega" to "ω"
    )

    private val textWrapperRegex = Regex("""\\(?:text|mathrm|mathbf|mathit|mathsf|mathtt|operatorname|pmb|boldsymbol|bm|bold)\{([^}]+)\}""")
    private val fracRegex = Regex("""\\frac\{([^}]+)\}\{([^}]+)\}""")
    private val sqrtNRegex = Regex("""\\sqrt\[([^\]]+)\]\{([^}]+)\}""")
    private val sqrtRegex = Regex("""\\sqrt\{([^}]+)\}""")
    private val supGroupRegex = Regex("""\^\{([^}]+)\}""")
    private val supSingleRegex = Regex("""\^([a-zA-Z0-9+\-=()])""")
    private val subGroupRegex = Regex("""_\{([^}]+)\}""")
    private val subSingleRegex = Regex("""_([a-zA-Z0-9+\-=()])""")
    private val extensibleArrowRegex = Regex("""\\x(rightarrow|leftarrow|Rightarrow|Leftarrow|leftrightarrow|Leftrightarrow|mapsto)(?:\[([^\]]*)\])?\{([^}]*)\}""")
    private val oversetRegex = Regex("""\\overset\{([^}]*)\}\{([^}]*)\}""")
    private val undersetRegex = Regex("""\\underset\{([^}]*)\}\{([^}]*)\}""")
    private val boxedRegex = Regex("""\\boxed\{([^}]*)\}""")
    private val mathEnvRegex = Regex("""\\(?:begin|end)\{(?:aligned|matrix|bmatrix|pmatrix|vmatrix|cases|array|equation\*?)\}""")
    private val mathTagRegex = Regex("""\\tag\{([^}]*)\}""")
    private val mathLabelRegex = Regex("""\\label\{([^}]*)\}""")

    private val blockRegex = Regex("""```[a-zA-Z0-9_\-]*\n[\s\S]*?```""")
    private val inlineRegex = Regex("""`[^`\n]+`""")
    private val blockMathRegex = Regex("""\$\$([\s\S]+?)\$\$|\\\[([\s\S]+?)\\\]""")
    private val inlineMathRegex = Regex("""(?<!\\)\$(?!\s)([^$\n]+?)(?<!\s)(?<!\\)\$|\\\(([\s\S]+?)\\\)""")

    private val symbolLookup: Map<String, String> by lazy {
        val map = mutableMapOf<String, String>()
        for ((pattern, replacement) in symbols) {
            val cmd = pattern.removePrefix("\\")
            map[cmd] = replacement
        }
        map
    }

    private val wordBoundaryCommands = setOf("to", "gets", "le", "ge", "ne", "in", "ni", "empty", "sim")

    private fun replaceSymbolsSinglePass(input: String): String {
        if (!input.contains('\\')) return input
        val sb = StringBuilder(input.length)
        var i = 0
        val n = input.length
        while (i < n) {
            val ch = input[i]
            if (ch == '\\' && i + 1 < n && ((input[i + 1] in 'a'..'z') || (input[i + 1] in 'A'..'Z'))) {
                var j = i + 1
                while (j < n && ((input[j] in 'a'..'z') || (input[j] in 'A'..'Z'))) j++
                val cmdName = input.substring(i + 1, j)
                var fullKey = cmdName
                if (j < n && input[j] == '{') {
                    val closeBrace = input.indexOf('}', j + 1)
                    if (closeBrace != -1) {
                        val paramKey = cmdName + input.substring(j, closeBrace + 1)
                        if (symbolLookup.containsKey(paramKey)) {
                            fullKey = paramKey
                            j = closeBrace + 1
                        }
                    }
                }
                val replacement = symbolLookup[fullKey]
                if (replacement != null) {
                    if (wordBoundaryCommands.contains(fullKey) && j < n && ((input[j] in 'a'..'z') || (input[j] in 'A'..'Z'))) {
                        sb.append(ch)
                        i++
                    } else {
                        sb.append(replacement)
                        i = j
                    }
                } else {
                    sb.append(ch)
                    i++
                }
            } else {
                sb.append(ch)
                i++
            }
        }
        return sb.toString()
    }

    /**
     * Translates dynamic extensible arrows like \xrightarrow{text} into clean native Unicode typography.
     */
    fun replaceExtensibleArrows(input: String): String {
        if (!input.contains("\\x")) return input
        return extensibleArrowRegex.replace(input) { match ->
            val arrowType = match.groupValues[1]
            val rawSub = match.groups[2]?.value
            val rawSup = match.groupValues[3]
            val sup = cleanArrowLabel(rawSup)
            val sub = if (rawSub != null) cleanArrowLabel(rawSub) else ""

            when (arrowType) {
                "rightarrow" -> when {
                    sup.isNotEmpty() && sub.isNotEmpty() -> "──($sup / $sub)──>"
                    sup.isNotEmpty() -> "──($sup)──>"
                    sub.isNotEmpty() -> "──($sub)──>"
                    else -> "⟶"
                }
                "leftarrow" -> when {
                    sup.isNotEmpty() && sub.isNotEmpty() -> "<──($sup / $sub)──"
                    sup.isNotEmpty() -> "<──($sup)──"
                    sub.isNotEmpty() -> "<──($sub)──"
                    else -> "⟵"
                }
                "Rightarrow" -> when {
                    sup.isNotEmpty() && sub.isNotEmpty() -> "══($sup / $sub)══>"
                    sup.isNotEmpty() -> "══($sup)══>"
                    sub.isNotEmpty() -> "══($sub)══>"
                    else -> "⟹"
                }
                "Leftarrow" -> when {
                    sup.isNotEmpty() && sub.isNotEmpty() -> "<══($sup / $sub)══"
                    sup.isNotEmpty() -> "<══($sup)══"
                    sub.isNotEmpty() -> "<══($sub)══"
                    else -> "⟸"
                }
                "leftrightarrow" -> when {
                    sup.isNotEmpty() && sub.isNotEmpty() -> "<──($sup / $sub)──>"
                    sup.isNotEmpty() -> "<──($sup)──>"
                    sub.isNotEmpty() -> "<──($sub)──>"
                    else -> "⟷"
                }
                "Leftrightarrow" -> when {
                    sup.isNotEmpty() && sub.isNotEmpty() -> "<══($sup / $sub)══>"
                    sup.isNotEmpty() -> "<══($sup)══>"
                    sub.isNotEmpty() -> "<══($sub)══>"
                    else -> "⟺"
                }
                "mapsto" -> when {
                    sup.isNotEmpty() && sub.isNotEmpty() -> "|──($sup / $sub)──>"
                    sup.isNotEmpty() -> "|──($sup)──>"
                    sub.isNotEmpty() -> "|──($sub)──>"
                    else -> "⟼"
                }
                else -> "⟶"
            }
        }
    }

    private fun cleanArrowLabel(label: String): String {
        var s = label.trim()
        s = textWrapperRegex.replace(s, "$1")
        s = s.replace("{", "").replace("}", "")
        return s.trim()
    }

    /**
     * Translates an isolated math expression into Unicode.
     */
    fun cleanMathExpression(input: String): String {
        var str = input

        // 1. Text wrappers: \text{...}, \mathrm{...}, \mathbf{...}, \operatorname{...}, etc.
        str = textWrapperRegex.replace(str, "$1")

        // 2. Boxed: \boxed{x} -> [x]
        str = boxedRegex.replace(str, "[$1]")

        // 3. Overset / Underset: \overset{a}{b} -> b (a)
        str = oversetRegex.replace(str, "$2 ($1)")
        str = undersetRegex.replace(str, "$2 ($1)")

        // 4. Extensible arrows: \xrightarrow{训练} -> ──(训练)──>
        str = replaceExtensibleArrows(str)

        // 5. Math tag and label
        str = mathTagRegex.replace(str, "($1)")
        str = mathLabelRegex.replace(str, "")

        // 6. Math environments
        str = mathEnvRegex.replace(str, "")

        // 7. Fractions: \frac{a}{b} -> a / b
        str = fracRegex.replace(str, "$1 / $2")

        // 8. Square roots: \sqrt[n]{x} -> ⁿ√(x), \sqrt{x} -> √(x)
        str = sqrtNRegex.replace(str) { match ->
            val n = match.groupValues[1].map { superscriptMap[it] ?: it }.joinToString("")
            val inner = match.groupValues[2]
            "${n}√($inner)"
        }
        str = sqrtRegex.replace(str, "√($1)")

        // 9. Bracket cleanups
        str = str
            .replace("\\left(", "(")
            .replace("\\right)", ")")
            .replace("\\left[", "[")
            .replace("\\right]", "]")
            .replace("\\left\\{", "{")
            .replace("\\right\\}", "}")
            .replace("\\{", "{")
            .replace("\\}", "}")
            .replace("\\%", "%")
            .replace("\\_", "_")
            .replace("\\&", "&")
            .replace("\\,", " ")
            .replace("\\;", " ")
            .replace("\\quad", " ")
            .replace("\\qquad", "  ")

        // 10. Replace LaTeX symbols (single-pass scanner)
        str = replaceSymbolsSinglePass(str)

        // 11. Convert superscripts
        if (str.contains("^")) {
            str = supGroupRegex.replace(str) { match ->
                match.groupValues[1].map { superscriptMap[it] ?: it }.joinToString("")
            }
            str = supSingleRegex.replace(str) { match ->
                val char = match.groupValues[1].firstOrNull()
                superscriptMap[char]?.toString() ?: match.value
            }
        }

        // 12. Convert subscripts
        if (str.contains("_")) {
            str = subGroupRegex.replace(str) { match ->
                match.groupValues[1].map { subscriptMap[it] ?: it }.joinToString("")
            }
            str = subSingleRegex.replace(str) { match ->
                val char = match.groupValues[1].firstOrNull()
                subscriptMap[char]?.toString() ?: match.value
            }
        }

        return str.trim()
    }

    private final class ProcessedMathCache {
        private val lock = Any()
        private val cache = LinkedHashMap<Int, String>(100, 0.75f, true)

        fun get(hash: Int): String? {
            synchronized(lock) {
                return cache[hash]
            }
        }

        fun set(hash: Int, value: String) {
            synchronized(lock) {
                if (cache.size > 500) {
                    val it = cache.iterator()
                    if (it.hasNext()) {
                        it.next()
                        it.remove()
                    }
                }
                cache[hash] = value
            }
        }
    }

    private val cache = ProcessedMathCache()

    /**
     * Processes full document or paragraph text:
     * Replaces $$...$$, \[...\], $...$, and \(...\) blocks with clean Unicode math.
     * Also replaces common standalone LaTeX arrows and symbols outside formulas,
     * safely protecting code blocks and inline code spans.
     */
    fun process(text: String): String {
        if (!text.contains("$") && !text.contains("\\")) return text

        val hash = text.hashCode()
        cache.get(hash)?.let { return it }

        var protectedText = text

        // 1. Protect fenced code blocks ```...```
        val codeBlocks = mutableListOf<String>()
        protectedText = blockRegex.replace(protectedText) { match ->
            val token = "XXAGYBLOCKTOKEN${codeBlocks.size}XX"
            codeBlocks.add(match.value)
            token
        }

        // 2. Protect inline code spans `code`
        val inlineSpans = mutableListOf<String>()
        protectedText = inlineRegex.replace(protectedText) { match ->
            val token = "XXAGYINLINETOKEN${inlineSpans.size}XX"
            inlineSpans.add(match.value)
            token
        }

        // 3. Process block math $$...$$ or \[...\]
        protectedText = blockMathRegex.replace(protectedText) { match ->
            var formula = match.groupValues[1].ifEmpty { match.groupValues[2] }
                .trim('$', ' ', '\t', '\n', '\r')
                .replace("\\[", "")
                .replace("\\]", "")
                .replace("\\\\", "\n")
            cleanMathExpression(formula)
        }

        // 4. Process inline math $...$ or \(...\)
        protectedText = inlineMathRegex.replace(protectedText) { match ->
            var inner = match.value
            if (inner.startsWith("$") && inner.endsWith("$") && inner.length >= 2) {
                inner = inner.substring(1, inner.length - 1)
            } else if (inner.startsWith("\\(") && inner.endsWith("\\)") && inner.length >= 4) {
                inner = inner.substring(2, inner.length - 2)
            }
            cleanMathExpression(inner)
        }

        // 5. Transform standalone extensible arrows, boxed, overset, and LaTeX symbols in prose
        protectedText = replaceExtensibleArrows(protectedText)
        protectedText = boxedRegex.replace(protectedText, "[$1]")
        protectedText = oversetRegex.replace(protectedText, "$2 ($1)")
        protectedText = undersetRegex.replace(protectedText, "$2 ($1)")
        protectedText = replaceSymbolsSinglePass(protectedText)

        // 6. Restore inline code spans
        for (idx in inlineSpans.indices) {
            val token = "XXAGYINLINETOKEN${idx}XX"
            protectedText = protectedText.replace(token, inlineSpans[idx])
        }

        // 7. Restore fenced code blocks
        for (idx in codeBlocks.indices) {
            val token = "XXAGYBLOCKTOKEN${idx}XX"
            protectedText = protectedText.replace(token, codeBlocks[idx])
        }

        cache.set(hash, protectedText)
        return protectedText
    }
}
