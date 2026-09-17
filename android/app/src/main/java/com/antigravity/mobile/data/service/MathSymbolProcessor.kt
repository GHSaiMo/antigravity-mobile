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

    private val textWrapperRegex = Regex("""\\(?:text|mathrm|mathbf|mathit|mathsf|mathtt)\{([^}]+)\}""")
    private val fracRegex = Regex("""\\frac\{([^}]+)\}\{([^}]+)\}""")
    private val sqrtNRegex = Regex("""\\sqrt\[([^\]]+)\]\{([^}]+)\}""")
    private val sqrtRegex = Regex("""\\sqrt\{([^}]+)\}""")
    private val supGroupRegex = Regex("""\^\{([^}]+)\}""")
    private val supSingleRegex = Regex("""\^([a-zA-Z0-9+\-=()])""")
    private val subGroupRegex = Regex("""_\{([^}]+)\}""")
    private val subSingleRegex = Regex("""_([a-zA-Z0-9+\-=()])""")

    // Formula wrapper regexes: $$...$$, $...$, \[...\], \(...\)
    private val blockMathRegex = Regex("""\$\$([\s\S]+?)\$\$|\\\[([\s\S]+?)\\\]""")
    private val inlineMathRegex = Regex("""(?<!\$)\$(?!\$)(.+?)(?<!\$)\$(?!\$)|\\\(([\s\S]+?)\\\)""")

    /**
     * Translates an isolated math expression into Unicode.
     */
    fun cleanMathExpression(input: String): String {
        var str = input

        // 1. Text wrappers: \text{...} -> ...
        str = textWrapperRegex.replace(str, "$1")

        // 2. Fractions: \frac{a}{b} -> a / b
        str = fracRegex.replace(str, "$1 / $2")

        // 3. Square roots: \sqrt[n]{x} -> ⁿ√(x), \sqrt{x} -> √(x)
        str = sqrtNRegex.replace(str) { match ->
            val n = match.groupValues[1].map { superscriptMap[it] ?: it }.joinToString("")
            val inner = match.groupValues[2]
            "${n}√($inner)"
        }
        str = sqrtRegex.replace(str, "√($1)")

        // 4. Bracket cleanups
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

        // 5. Replace LaTeX symbols
        for ((pattern, replacement) in symbols) {
            str = str.replace(pattern, replacement)
        }

        // 6. Convert superscripts
        if (str.contains("^")) {
            str = supGroupRegex.replace(str) { match ->
                match.groupValues[1].map { superscriptMap[it] ?: it }.joinToString("")
            }
            str = supSingleRegex.replace(str) { match ->
                val char = match.groupValues[1].firstOrNull()
                superscriptMap[char]?.toString() ?: match.value
            }
        }

        // 7. Convert subscripts
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

    /**
     * Processes full document or paragraph text:
     * Replaces $$...$$, \[...\], $...$, and \(...\) blocks with clean Unicode math.
     * Also replaces common standalone LaTeX arrows and symbols outside formulas.
     */
    fun process(text: String): String {
        if (!text.contains("$") && !text.contains("\\")) return text

        var result = text

        // 1. Process block math $$...$$ or \[...\]
        result = blockMathRegex.replace(result) { match ->
            val formula = match.groupValues[1].ifEmpty { match.groupValues[2] }
            cleanMathExpression(formula)
        }

        // 2. Process inline math $...$ or \(...\)
        result = inlineMathRegex.replace(result) { match ->
            val formula = match.groupValues[1].ifEmpty { match.groupValues[2] }
            cleanMathExpression(formula)
        }

        // 3. Clean remaining common standalone arrows or comparisons
        result = result
            .replace("\\to", "→")
            .replace("\\gets", "←")
            .replace("\\implies", "⇒")
            .replace("\\iff", "⇔")
            .replace("\\le", "≤")
            .replace("\\ge", "≥")
            .replace("\\ne", "≠")

        return result
    }
}
