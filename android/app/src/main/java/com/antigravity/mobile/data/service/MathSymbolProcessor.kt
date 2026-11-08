package com.antigravity.mobile.data.service

/**
 * 1:1 Kotlin port of iOS MathSymbolProcessor.swift.
 * Comprehensive LaTeX math and mathematical symbol processor.
 * Converts LaTeX formulas, inline/block math, arrows, operators, Greek letters,
 * superscripts, subscripts, and structural commands into clean, native Unicode typography.
 */
object MathSymbolProcessor {

    private val superscriptMap: Map<Char, String> = mapOf(
        '0' to "⁰", '1' to "¹", '2' to "²", '3' to "³", '4' to "⁴",
        '5' to "⁵", '6' to "⁶", '7' to "⁷", '8' to "⁸", '9' to "⁹",
        '+' to "⁺", '-' to "⁻", '=' to "⁼", '(' to "⁽", ')' to "⁾",
        'n' to "ⁿ", 'i' to "ⁱ", 'j' to "ʲ", 'a' to "ᵃ", 'b' to "ᵇ",
        'c' to "ᶜ", 'd' to "ᵈ", 'e' to "ᵉ", 'f' to "ᶠ", 'g' to "ᵍ",
        'h' to "ʰ", 'k' to "ᵏ", 'l' to "ˡ", 'm' to "ᵐ", 'o' to "ᵒ",
        'p' to "ᵖ", 'r' to "ʳ", 's' to "ˢ", 't' to "ᵗ", 'u' to "ᵘ",
        'v' to "ᵛ", 'w' to "ʷ", 'x' to "ˣ", 'y' to "ʸ", 'z' to "ᶻ",
        'A' to "ᴬ", 'B' to "ᴮ", 'D' to "ᴰ", 'E' to "ᴱ", 'G' to "ᴳ",
        'H' to "ᴴ", 'I' to "ᴵ", 'J' to "ᴶ", 'K' to "ᴷ", 'L' to "ᴸ",
        'M' to "ᴹ", 'N' to "ᴺ", 'O' to "ᴼ", 'P' to "ᴾ", 'R' to "ᴿ",
        'T' to "ᵀ", 'U' to "ᵁ", 'V' to "ⱽ", 'W' to "ᵂ",
        '⊤' to "ᵀ", '∘' to "°", '′' to "′", '*' to "*", '∗' to "*"
    )

    private val subscriptMap: Map<Char, String> = mapOf(
        '0' to "₀", '1' to "₁", '2' to "₂", '3' to "₃", '4' to "₄",
        '5' to "₅", '6' to "₆", '7' to "₇", '8' to "⁸", '9' to "₉",
        '+' to "₊", '-' to "₋", '=' to "₌", '(' to "₍", ')' to "₎",
        'a' to "ₐ", 'e' to "ₑ", 'h' to "ₕ", 'i' to "ᵢ", 'j' to "ⱼ",
        'k' to "ₖ", 'l' to "ₗ", 'm' to "ₘ", 'n' to "ₙ", 'o' to "ₒ",
        'p' to "ₚ", 'r' to "ᵣ", 's' to "ₛ", 't' to "ₜ", 'u' to "ᵤ",
        'v' to "ᵥ", 'x' to "ₓ",
        'β' to "ᵦ", 'γ' to "ᵧ", 'ρ' to "ᵨ", 'φ' to "ᵩ", 'χ' to "ᵪ"
    )

    // Symbol dictionary
    private val symbols = listOf(
        // Blackboard bold (\mathbb & \mathbbm)
        "\\mathbb{A}" to "𝔸", "\\mathbb{B}" to "𝔹", "\\mathbb{C}" to "ℂ", "\\mathbf{C}" to "ℂ",
        "\\mathbb{D}" to "𝔻", "\\mathbb{E}" to "𝔼", "\\mathbb{F}" to "𝔽", "\\mathbb{G}" to "𝔾",
        "\\mathbb{H}" to "ℍ", "\\mathbb{I}" to "𝕀", "\\mathbb{J}" to "𝕁", "\\mathbb{K}" to "𝕂",
        "\\mathbb{L}" to "𝕃", "\\mathbb{M}" to "𝕄", "\\mathbb{N}" to "ℕ", "\\mathbf{N}" to "ℕ",
        "\\mathbb{O}" to "𝕆", "\\mathbb{P}" to "ℙ", "\\mathbb{Q}" to "ℚ", "\\mathbf{Q}" to "ℚ",
        "\\mathbb{R}" to "ℝ", "\\mathbf{R}" to "ℝ", "\\mathbb{S}" to "𝕊", "\\mathbb{T}" to "𝕋",
        "\\mathbb{U}" to "𝕌", "\\mathbb{V}" to "𝕍", "\\mathbb{W}" to "𝕎", "\\mathbb{X}" to "𝕏",
        "\\mathbb{Y}" to "𝕐", "\\mathbb{Z}" to "ℤ", "\\mathbf{Z}" to "ℤ",
        "\\mathbb{0}" to "𝟘", "\\mathbb{1}" to "𝟙", "\\mathbbm{1}" to "𝟙", "\\mathbb{k}" to "𝕜",

        // Calligraphic (\mathcal) - full A..Z
        "\\mathcal{A}" to "𝒜", "\\mathcal{B}" to "ℬ", "\\mathcal{C}" to "𝒞", "\\mathcal{D}" to "𝒟",
        "\\mathcal{E}" to "ℰ", "\\mathcal{F}" to "ℱ", "\\mathcal{G}" to "𝒢", "\\mathcal{H}" to "ℋ",
        "\\mathcal{I}" to "ℐ", "\\mathcal{J}" to "𝒥", "\\mathcal{K}" to "𝒦", "\\mathcal{L}" to "ℒ",
        "\\mathcal{M}" to "ℳ", "\\mathcal{N}" to "𝒩", "\\mathcal{O}" to "𝒪", "\\mathcal{P}" to "𝒫",
        "\\mathcal{Q}" to "𝒬", "\\mathcal{R}" to "ℛ", "\\mathcal{S}" to "𝒮", "\\mathcal{T}" to "𝒯",
        "\\mathcal{U}" to "𝒰", "\\mathcal{V}" to "𝒱", "\\mathcal{W}" to "𝒲", "\\mathcal{X}" to "𝒳",
        "\\mathcal{Y}" to "𝒴", "\\mathcal{Z}" to "𝒵",

        // Script (\mathscr) - full A..Z
        "\\mathscr{A}" to "𝒜", "\\mathscr{B}" to "ℬ", "\\mathscr{C}" to "𝒞", "\\mathscr{D}" to "𝒟",
        "\\mathscr{E}" to "ℰ", "\\mathscr{F}" to "ℱ", "\\mathscr{G}" to "𝒢", "\\mathscr{H}" to "ℋ",
        "\\mathscr{I}" to "ℐ", "\\mathscr{J}" to "𝒥", "\\mathscr{K}" to "𝒦", "\\mathscr{L}" to "ℒ",
        "\\mathscr{M}" to "ℳ", "\\mathscr{N}" to "𝒩", "\\mathscr{O}" to "𝒪", "\\mathscr{P}" to "𝒫",
        "\\mathscr{Q}" to "𝒬", "\\mathscr{R}" to "ℛ", "\\mathscr{S}" to "𝒮", "\\mathscr{T}" to "𝒯",
        "\\mathscr{U}" to "𝒰", "\\mathscr{V}" to "𝒱", "\\mathscr{W}" to "𝒲", "\\mathscr{X}" to "𝒳",
        "\\mathscr{Y}" to "𝒴", "\\mathscr{Z}" to "𝒵",

        // Fraktur (\mathfrak) - A..Z, a..z
        "\\mathfrak{A}" to "𝔄", "\\mathfrak{B}" to "𝔅", "\\mathfrak{C}" to "ℭ", "\\mathfrak{D}" to "𝔇",
        "\\mathfrak{E}" to "𝔈", "\\mathfrak{F}" to "𝔉", "\\mathfrak{G}" to "𝔊", "\\mathfrak{H}" to "ℌ",
        "\\mathfrak{I}" to "ℑ", "\\mathfrak{J}" to "𝔍", "\\mathfrak{K}" to "𝔎", "\\mathfrak{L}" to "𝔏",
        "\\mathfrak{M}" to "𝔐", "\\mathfrak{N}" to "𝔑", "\\mathfrak{O}" to "𝔒", "\\mathfrak{P}" to "𝔓",
        "\\mathfrak{Q}" to "𝔔", "\\mathfrak{R}" to "ℜ", "\\mathfrak{S}" to "𝔖", "\\mathfrak{T}" to "𝔗",
        "\\mathfrak{U}" to "𝔘", "\\mathfrak{V}" to "𝔙", "\\mathfrak{W}" to "𝔚", "\\mathfrak{X}" to "𝔛",
        "\\mathfrak{Y}" to "𝔜", "\\mathfrak{Z}" to "ℨ",
        "\\mathfrak{a}" to "𝔞", "\\mathfrak{b}" to "𝔟", "\\mathfrak{c}" to "𝔠", "\\mathfrak{d}" to "𝔡",
        "\\mathfrak{e}" to "𝔢", "\\mathfrak{f}" to "𝔣", "\\mathfrak{g}" to "𝔤", "\\mathfrak{h}" to "𝔥",
        "\\mathfrak{i}" to "𝔦", "\\mathfrak{j}" to "𝔧", "\\mathfrak{k}" to "𝔨", "\\mathfrak{l}" to "𝔩",
        "\\mathfrak{m}" to "𝔪", "\\mathfrak{n}" to "𝔫", "\\mathfrak{o}" to "𝔬", "\\mathfrak{p}" to "𝔭",
        "\\mathfrak{q}" to "𝔮", "\\mathfrak{r}" to "𝔯", "\\mathfrak{s}" to "𝔰", "\\mathfrak{t}" to "𝔱",
        "\\mathfrak{u}" to "𝔲", "\\mathfrak{v}" to "𝔳", "\\mathfrak{w}" to "𝔴", "\\mathfrak{x}" to "𝔵",
        "\\mathfrak{y}" to "𝔶", "\\mathfrak{z}" to "𝔷",

        // Delimiters, Brackets & Norms
        "\\langle" to "⟨",
        "\\rangle" to "⟩",
        "\\lfloor" to "⌊",
        "\\rfloor" to "⌋",
        "\\lceil" to "⌈",
        "\\rceil" to "⌉",
        "\\ulcorner" to "⌜",
        "\\urcorner" to "⌝",
        "\\llcorner" to "⌞",
        "\\lrcorner" to "⌟",
        "\\lVert" to "‖",
        "\\rVert" to "‖",
        "\\Vert" to "‖",
        "\\lvert" to "|",
        "\\rvert" to "|",
        "\\vert" to "|",
        "\\mid" to "∣",
        "\\nmid" to "∤",
        "\\backslash" to "\\",

        // Long & squiggly arrows
        "\\longleftrightarrow" to "⟷",
        "\\Longleftrightarrow" to "⟺",
        "\\leftrightsquigarrow" to "↭",
        "\\rightsquigarrow" to "⇝",
        "\\twoheadrightarrow" to "↠",
        "\\twoheadleftarrow" to "↞",
        "\\longrightarrow" to "⟶",
        "\\longleftarrow" to "⟵",
        "\\Longrightarrow" to "⟹",
        "\\Longleftarrow" to "⟸",
        "\\rightarrowtail" to "↣",
        "\\leftarrowtail" to "↢",
        "\\circlearrowleft" to "↺",
        "\\circlearrowright" to "↻",
        "\\curvearrowleft" to "↶",
        "\\curvearrowright" to "↷",
        "\\longmapsto" to "⟼",
        "\\mapsfrom" to "↤",
        "\\leadsto" to "↝",

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
        "\\impliedby" to "⇐",
        "\\iff" to "⇔",
        "\\to" to "→",
        "\\gets" to "←",

        // Comparisons & Relations
        "\\fallingdotseq" to "≒",
        "\\risingdotseq" to "≓",
        "\\lessapprox" to "⪅",
        "\\gtrapprox" to "⪆",
        "\\sqsubseteq" to "⊑",
        "\\sqsupseteq" to "⊒",
        "\\sqsubset" to "⊏",
        "\\sqsupset" to "⊐",
        "\\triangleq" to "≜",
        "\\coloneqq" to "≔",
        "\\eqqcolon" to "≕",
        "\\coloneq" to "≔",
        "\\eqcolon" to "≕",
        "\\defeq" to "≔",
        "\\leqslant" to "≤",
        "\\geqslant" to "≥",
        "\\subseteq" to "⊆",
        "\\supseteq" to "⊇",
        "\\subsetneq" to "⊊",
        "\\supsetneq" to "⊋",
        "\\nsubseteq" to "⊈",
        "\\nsupseteq" to "⊉",
        "\\nsubset" to "⊄",
        "\\parallel" to "∥",
        "\\nparallel" to "∦",
        "\\lesssim" to "≲",
        "\\gtrsim" to "≳",
        "\\napprox" to "≉",
        "\\nequiv" to "≢",
        "\\circeq" to "≗",
        "\\preceq" to "⪯",
        "\\succeq" to "⪰",
        "\\approx" to "≈",
        "\\simeq" to "≃",
        "\\ncong" to "≇",
        "\\cong" to "≅",
        "\\equiv" to "≡",
        "\\propto" to "∝",
        "\\asymp" to "≍",
        "\\doteq" to "≐",
        "\\bowtie" to "⋈",
        "\\ltimes" to "⋉",
        "\\rtimes" to "⋊",
        "\\prec" to "≺",
        "\\succ" to "≻",
        "\\nsim" to "≁",
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
        "\\smallsetminus" to "∖",
        "\\complement" to "∁",
        "\\varnothing" to "∅",
        "\\emptyset" to "∅",
        "\\empty" to "∅",
        "\\setminus" to "∖",
        "\\therefore" to "∴",
        "\\because" to "∵",
        "\\notin" to "∉",
        "\\subset" to "⊂",
        "\\supset" to "⊃",
        "\\forall" to "∀",
        "\\exists" to "∃",
        "\\nexists" to "∄",
        "\\lnot" to "¬",
        "\\land" to "∧",
        "\\wedge" to "∧",
        "\\lor" to "∨",
        "\\vee" to "∨",
        "\\top" to "⊤",
        "\\bot" to "⊥",
        "\\vdash" to "⊢",
        "\\dashv" to "⊣",
        "\\vDash" to "⊨",
        "\\Vdash" to "⊩",
        "\\models" to "⊨",
        "\\bigcup" to "⋃",
        "\\bigcap" to "⋂",
        "\\bigsqcup" to "⨆",
        "\\biguplus" to "⨄",
        "\\bigvee" to "⋁",
        "\\bigwedge" to "⋀",
        "\\sqcup" to "⊔",
        "\\sqcap" to "⊓",
        "\\uplus" to "⊎",
        "\\cup" to "∪",
        "\\cap" to "∩",
        "\\in" to "∈",
        "\\ni" to "∋",
        "\\owns" to "∋",
        "\\neg" to "¬",

        // Arithmetic & Operators
        "\\bigoplus" to "⨁",
        "\\bigotimes" to "⨂",
        "\\bigodot" to "⨀",
        "\\boxtimes" to "⊠",
        "\\boxplus" to "⊞",
        "\\boxminus" to "⊟",
        "\\boxdot" to "⊡",
        "\\times" to "×",
        "\\div" to "÷",
        "\\pm" to "±",
        "\\mp" to "∓",
        "\\cdot" to "·",
        "\\cdots" to "⋯",
        "\\ldots" to "…",
        "\\dotsc" to "…",
        "\\dotsb" to "⋯",
        "\\dotsm" to "⋯",
        "\\dotsi" to "⋯",
        "\\dots" to "…",
        "\\ddots" to "⋱",
        "\\vdots" to "⋮",
        "\\bullet" to "•",
        "\\circ" to "∘",
        "\\star" to "⋆",
        "\\ast" to "∗",
        "\\oplus" to "⊕",
        "\\ominus" to "⊖",
        "\\otimes" to "⊗",
        "\\oslash" to "⊘",
        "\\odot" to "⊙",
        "\\dagger" to "†",
        "\\ddagger" to "‡",
        "\\wr" to "≀",
        "\\colon" to ":",
        "\\prime" to "′",
        "\\checkmark" to "✓",

        // Calculus, Analysis & Geometry
        "\\oiiint" to "∰",
        "\\oiint" to "∯",
        "\\iiint" to "∭",
        "\\iint" to "∬",
        "\\oint" to "∮",
        "\\int" to "∫",
        "\\sum" to "∑",
        "\\prod" to "∏",
        "\\coprod" to "∐",
        "\\partial" to "∂",
        "\\nabla" to "∇",
        "\\infty" to "∞",
        "\\surd" to "√",
        "\\sqrt" to "√",
        "\\angle" to "∠",
        "\\measuredangle" to "∡",
        "\\sphericalangle" to "∢",
        "\\blacktriangledown" to "▼",
        "\\bigtriangledown" to "▽",
        "\\blacktriangle" to "▲",
        "\\bigtriangleup" to "△",
        "\\triangleleft" to "◁",
        "\\triangleright" to "▷",
        "\\triangle" to "△",
        "\\blacksquare" to "■",
        "\\diamond" to "◇",
        "\\Diamond" to "◇",
        "\\lozenge" to "◊",
        "\\square" to "□",
        "\\Box" to "□",
        "\\degree" to "°",
        "\\hbar" to "ℏ",
        "\\ell" to "ℓ",
        "\\aleph" to "ℵ",
        "\\beth" to "ℶ",
        "\\gimel" to "ℷ",
        "\\daleth" to "ℸ",
        "\\wp" to "℘",
        "\\Re" to "ℜ",
        "\\Im" to "ℑ",
        "\\mho" to "℧",

        // Standard Math Functions
        "\\limsup" to "lim sup",
        "\\liminf" to "lim inf",
        "\\argmax" to "arg max",
        "\\argmin" to "arg min",
        "\\arcsin" to "arcsin",
        "\\arccos" to "arccos",
        "\\arctan" to "arctan",
        "\\sinh" to "sinh",
        "\\cosh" to "cosh",
        "\\tanh" to "tanh",
        "\\coth" to "coth",
        "\\lim" to "lim",
        "\\sup" to "sup",
        "\\inf" to "inf",
        "\\max" to "max",
        "\\min" to "min",
        "\\arg" to "arg",
        "\\det" to "det",
        "\\dim" to "dim",
        "\\ker" to "ker",
        "\\hom" to "hom",
        "\\deg" to "deg",
        "\\gcd" to "gcd",
        "\\lcm" to "lcm",
        "\\exp" to "exp",
        "\\log" to "log",
        "\\ln" to "ln",
        "\\lg" to "lg",
        "\\sin" to "sin",
        "\\cos" to "cos",
        "\\tan" to "tan",
        "\\cot" to "cot",
        "\\sec" to "sec",
        "\\csc" to "csc",
        "\\Pr" to "Pr",
        "\\bmod" to "mod",

        // Layout / Spacing / Style No-ops
        "\\displaystyle" to "",
        "\\scriptscriptstyle" to "",
        "\\scriptstyle" to "",
        "\\textstyle" to "",
        "\\nolimits" to "",
        "\\limits" to "",
        "\\noindent" to "",
        "\\qquad" to "  ",
        "\\quad" to " ",
        "\\enspace" to " ",
        "\\thinspace" to " ",

        // Greek uppercase
        "\\Gamma" to "Γ", "\\Delta" to "Δ", "\\Theta" to "Θ",
        "\\Lambda" to "Λ", "\\Xi" to "Ξ", "\\Pi" to "Π",
        "\\Sigma" to "Σ", "\\Upsilon" to "Υ", "\\Phi" to "Φ",
        "\\Psi" to "Ψ", "\\Omega" to "Ω",

        // Greek lowercase
        "\\varepsilon" to "ε", "\\vartheta" to "ϑ", "\\varpi" to "ϖ",
        "\\varrho" to "ϱ", "\\varsigma" to "ς", "\\varphi" to "φ",
        "\\alpha" to "α", "\\beta" to "β", "\\gamma" to "γ",
        "\\delta" to "δ", "\\epsilon" to "ϵ", "\\zeta" to "ζ",
        "\\eta" to "η", "\\theta" to "θ", "\\iota" to "ι",
        "\\kappa" to "κ", "\\lambda" to "λ", "\\mu" to "μ",
        "\\nu" to "ν", "\\xi" to "ξ", "\\omicron" to "ο",
        "\\pi" to "π", "\\rho" to "ρ", "\\sigma" to "σ",
        "\\tau" to "τ", "\\upsilon" to "υ", "\\phi" to "ϕ",
        "\\chi" to "χ", "\\psi" to "ψ", "\\omega" to "ω"
    )

    private val subSupTextWrapperRegex = Regex("""([_^])\\(?:text|textbf|textit|textrm|textsf|texttt|textsc|mathrm|mathbf|mathit|mathsf|mathtt|mathbbm|operatorname\*?|pmb|boldsymbol|bm|bold|emph|mbox|hbox)\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}""")
    private val textWrapperRegex = Regex("""\\(?:text|textbf|textit|textrm|textsf|texttt|textsc|mathrm|mathbf|mathit|mathsf|mathtt|mathbbm|operatorname\*?|pmb|boldsymbol|bm|bold|emph|mbox|hbox)\{([^{}]*)\}""")
    private val fontFallbackRegex = Regex("""\\(?:mathcal|mathscr|mathbb|mathfrak|mathbbm)\{([^{}]*)\}""")
    private val colorWrapperRegex = Regex("""\\(?:textcolor|color)\{[^{}]*\}\{([^{}]*)\}|\\color\{[^{}]*\}""")
    private val cancelRegex = Regex("""\\(?:cancel|bcancel|xcancel|sout)\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}""")
    private val pmodRegex = Regex("""\\pmod\{([^{}]*)\}""")
    private val binomRegex = Regex("""\\(?:d|t)?binom\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}""")
    private val underbraceRegex = Regex("""\\underbrace\s*\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}\s*(?:_\s*\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}|_\s*([a-zA-Z0-9])|\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\})?""")
    private val overbraceRegex = Regex("""\\overbrace\s*\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}\s*(?:\^\s*\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}|\^\s*([a-zA-Z0-9])|\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\})?""")
    private val fracRegex = Regex("""\\(?:d|t|c)?frac\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}""")
    private val sqrtNRegex = Regex("""\\sqrt\[([^\]]*)\]\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}""")
    private val sqrtRegex = Regex("""\\sqrt\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}""")
    private val accentRegex = Regex("""\\(widehat|hat|widetilde|tilde|overline|bar|overrightarrow|vec|ddot|dot|underline)\{([^{}]*)\}""")
    private val sizingPrefixRegex = Regex("""\\(?:left|right|middle|bigl|bigr|Bigl|Bigr|biggl|biggr|Biggl|Biggr|big|Big|bigg|Bigg)\s*(?=[(\[{|⟨⟩⌊⌋⌈⌉‖/\\])""")
    private val supGroupRegex = Regex("""\^\{([^{}]+)\}""")
    private val supSingleRegex = Regex("""\^([0-9a-zA-Z+\-*⊤∘′∗])""")
    private val subGroupRegex = Regex("""_\{([^{}]+)\}""")
    private val subSingleRegex = Regex("""_([0-9a-zA-Zβγρφχ])""")
    private val extensibleArrowRegex = Regex("""\\x(rightarrow|leftarrow|Rightarrow|Leftarrow|leftrightarrow|Leftrightarrow|mapsto)(?:\[([^\]]*)\])?\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}""")
    private val oversetRegex = Regex("""\\overset\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}""")
    private val undersetRegex = Regex("""\\underset\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}""")
    private val boxedRegex = Regex("""\\boxed\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}""")
    private val mathEnvRegex = Regex("""\\(?:begin|end)\{(?:aligned|align\*?|matrix|bmatrix|pmatrix|vmatrix|Vmatrix|Bmatrix|cases|split|gather\*?|multline\*?|array|equation\*?|subequations)\}(?:\{[^{}]*\})?""")
    private val mathTagRegex = Regex("""\\tag\*?\{([^{}]*)\}""")
    private val mathLabelRegex = Regex("""\\label\{([^{}]*)\}""")
    private val negatedOpRegex = Regex("""\\not\s*(\\(?:in|equiv|sim|approx|cong|subset|subseteq|supset|supseteq|parallel|mid)\b|=)""")
    private val bareOperatorLimitsRegex = Regex("""(\b(?:lim sup|lim inf|arg max|arg min|lim|max|min|sup|inf)|[∑∏∐∫∬∭∮∯∰⋃⋂⨁⨂⨀⨆⨄⋁⋀])\s*(?:_\{([^{}]+)\}|_([0-9a-zA-Z]))?(?:\s*(?:\^\{([^{}]+)\}|\^([0-9a-zA-Z+\-*⊤∘′∗])))?""")

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

    private val wordBoundaryCommands = setOf(
        "to", "gets", "le", "ge", "ne", "in", "ni", "empty", "sim",
        "mid", "ll", "gg", "cup", "cap", "neg", "top", "bot", "vee",
        "div", "pm", "mp", "ast", "wr", "int", "sum", "prod", "ell",
        "wp", "Re", "Im", "Box", "lim", "sup", "inf", "max", "min",
        "arg", "det", "dim", "ker", "hom", "deg", "gcd", "lcm", "exp",
        "log", "ln", "lg", "sin", "cos", "tan", "cot", "sec", "csc", "Pr"
    )

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

    private fun unwrapTextAndFontCommands(input: String): String {
        if (!input.contains('\\')) return input
        var str = subSupTextWrapperRegex.replace(input, "$1{$2}")
        str = colorWrapperRegex.replace(str, "$1")
        for (pass in 0 until 4) {
            val next = textWrapperRegex.replace(str, "$1")
            if (next == str) break
            str = next
        }
        return str
    }

    /**
     * Converts \underbrace{A}_{B}, \underbrace{A}{B}, \underbrace{A}_B, \underbrace{A} and \overbrace equivalents into A〔B〕.
     */
    fun replaceUnderOverBraces(input: String): String {
        if (!input.contains("brace")) return input
        var str = input

        fun applyBraceRegex(regex: Regex) {
            for (pass in 0 until 4) {
                if (!regex.containsMatchIn(str)) break
                str = regex.replace(str) { match ->
                    val base = match.groupValues[1].trim().replace(Regex("[ \\t]+"), " ")
                    val note = listOf(2, 3, 4)
                        .mapNotNull { idx -> match.groups[idx]?.value?.trim() }
                        .firstOrNull { it.isNotEmpty() }
                        .orEmpty()
                        .replace(Regex("[ \\t]+"), " ")
                    if (note.isEmpty()) base else "$base〔$note〕"
                }
            }
        }

        applyBraceRegex(underbraceRegex)
        applyBraceRegex(overbraceRegex)
        return str
    }

    private fun replaceBareOperatorLimits(input: String): String {
        if (!input.contains('_') && !input.contains('^')) return input
        return bareOperatorLimitsRegex.replace(input) { match ->
            val op = match.groupValues[1]
            val subGroup = match.groups[2]?.value
            val subSingle = match.groups[3]?.value
            val supGroup = match.groups[4]?.value
            val supSingle = match.groups[5]?.value
            if (subGroup == null && subSingle == null && supGroup == null && supSingle == null) {
                match.value
            } else {
                val subPart = when {
                    subGroup != null -> {
                        val inner = subGroup.trim()
                        val canMapAll = inner.isNotEmpty() && inner.all { subscriptMap.containsKey(it) || it == ' ' || it == ',' }
                        if (canMapAll) inner.map { subscriptMap[it] ?: it.toString() }.joinToString("") else "₍$inner₎"
                    }
                    subSingle != null -> {
                        val c = subSingle.firstOrNull()
                        if (c != null && subscriptMap.containsKey(c)) subscriptMap[c]!! else "₍$subSingle₎"
                    }
                    else -> ""
                }
                val supPart = when {
                    supGroup != null -> {
                        val inner = supGroup.trim()
                        val canMapAll = inner.isNotEmpty() && inner.all { superscriptMap.containsKey(it) || it == ' ' || it == ',' }
                        if (canMapAll) inner.map { superscriptMap[it] ?: it.toString() }.joinToString("") else "⁽$inner⁾"
                    }
                    supSingle != null -> {
                        val c = supSingle.firstOrNull()
                        if (c != null && superscriptMap.containsKey(c)) superscriptMap[c]!! else "⁽$supSingle⁾"
                    }
                    else -> ""
                }
                "$op$subPart$supPart"
            }
        }
    }

    private fun replaceStructuralMathCommands(input: String): String {
        if (!input.contains('\\')) return input
        var str = input

        if (str.contains("\\not")) {
            str = negatedOpRegex.replace(str) { match ->
                when (val op = match.groupValues[1]) {
                    "\\in" -> "∉"
                    "=" -> "≠"
                    "\\equiv" -> "≢"
                    "\\sim" -> "≁"
                    "\\approx" -> "≉"
                    "\\cong" -> "≇"
                    "\\subset" -> "⊄"
                    "\\subseteq" -> "⊈"
                    "\\supset" -> "⊅"
                    "\\supseteq" -> "⊉"
                    "\\parallel" -> "∦"
                    "\\mid" -> "∤"
                    else -> op
                }
            }
        }

        str = boxedRegex.replace(str, "[$1]")
        str = cancelRegex.replace(str, "~~$1~~")
        str = pmodRegex.replace(str, "(mod $1)")
        str = oversetRegex.replace(str, "$2 ($1)")
        str = undersetRegex.replace(str, "$2 ($1)")
        str = binomRegex.replace(str, "C($1, $2)")

        str = sqrtNRegex.replace(str) { match ->
            val n = match.groupValues[1].map { superscriptMap[it] ?: it.toString() }.joinToString("")
            val inner = match.groupValues[2]
            "${n}√($inner)"
        }
        for (pass in 0 until 2) {
            val next = sqrtRegex.replace(str, "√($1)")
            if (next == str) break
            str = next
        }
        for (pass in 0 until 3) {
            val next = fracRegex.replace(str, "$1 / $2")
            if (next == str) break
            str = next
        }

        str = accentRegex.replace(str) { match ->
            val cmd = match.groupValues[1]
            val base = match.groupValues[2].trim()
            val combining = when (cmd) {
                "hat", "widehat" -> "\u0302"
                "tilde", "widetilde" -> "\u0303"
                "bar", "overline" -> "\u0304"
                "vec", "overrightarrow" -> "\u20D7"
                "dot" -> "\u0307"
                "ddot" -> "\u0308"
                "underline" -> "\u0332"
                else -> ""
            }
            when {
                base.isEmpty() -> ""
                base.length == 1 || cmd == "overline" || cmd == "underline" ->
                    base.map { "$it$combining" }.joinToString("")
                else -> base + combining
            }
        }

        return str
    }

    private fun cleanSizingAndDelimiters(input: String, isMathMode: Boolean): String {
        var str = input
            .replace("\\left.", "")
            .replace("\\right.", "")
            .replace("\\left\\|", "‖")
            .replace("\\right\\|", "‖")
            .replace("\\left|", "|")
            .replace("\\right|", "|")
            .replace("\\left(", "(")
            .replace("\\right)", ")")
            .replace("\\left[", "[")
            .replace("\\right]", "]")
            .replace("\\left\\{", "{")
            .replace("\\right\\}", "}")
        str = sizingPrefixRegex.replace(str, "")
        if (isMathMode) {
            str = str
                .replace("\\|", "‖")
                .replace("\\{", "{")
                .replace("\\}", "}")
                .replace("\\%", "%")
                .replace("\\_", "_")
                .replace("\\,", " ")
                .replace("\\;", " ")
                .replace("\\:", " ")
                .replace("\\!", "")
                .replace("\\quad", " ")
                .replace("\\qquad", "  ")
        }
        return str
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
        var s = unwrapTextAndFontCommands(label.trim())
        s = replaceSymbolsSinglePass(s)
        s = s.replace("{", "").replace("}", "").replace(Regex("[ \\t]+"), " ")
        return s.trim()
    }

    /**
     * Translates an isolated math expression into Unicode.
     */
    fun cleanMathExpression(input: String): String {
        var str = input

        // 1. Unwrap \text{...}, \mathrm{...}, \mathbf{...}, \textcolor{...}{...}
        str = unwrapTextAndFontCommands(str)

        // 2. Underbrace & Overbrace: \underbrace{a}_{b} -> a〔b〕
        str = replaceUnderOverBraces(str)

        // 3. Extensible arrows: \xrightarrow{训练} -> ──(训练)──>
        str = replaceExtensibleArrows(str)

        // 4. Math tag, label, and environments
        str = mathTagRegex.replace(str, " ($1)")
        str = mathLabelRegex.replace(str, "")
        str = mathEnvRegex.replace(str, "")

        val escapedAmpPlaceholder = "\uE001"
        str = str.replace("\\&", escapedAmpPlaceholder)
            .replace("&=", "=")
            .replace("& =", "=")
            .replace("&", " ")
            .replace(escapedAmpPlaceholder, "&")

        // 5. Structural commands: \boxed, \overset, \underset, \binom, \sqrt, \frac, accents
        str = replaceStructuralMathCommands(str)

        // 6. Bracket & sizing cleanups
        str = cleanSizingAndDelimiters(str, isMathMode = true)

        // 7. Replace LaTeX symbols (single-pass scanner)
        str = replaceSymbolsSinglePass(str)
        str = fontFallbackRegex.replace(str, "$1")

        // 8. Convert superscripts
        if (str.contains("^")) {
            str = supGroupRegex.replace(str) { match ->
                val inner = match.groupValues[1].trim()
                val canMapAll = inner.isNotEmpty() && inner.all { superscriptMap.containsKey(it) || it == ' ' || it == ',' }
                if (canMapAll) {
                    inner.map { superscriptMap[it] ?: it.toString() }.joinToString("")
                } else {
                    "⁽$inner⁾"
                }
            }
            str = supSingleRegex.replace(str) { match ->
                val inner = match.groupValues[1]
                val char = inner.firstOrNull()
                if (char != null && superscriptMap.containsKey(char)) {
                    superscriptMap[char]!!
                } else {
                    "⁽$inner⁾"
                }
            }
        }

        // 9. Convert subscripts
        if (str.contains("_")) {
            str = subGroupRegex.replace(str) { match ->
                val inner = match.groupValues[1].trim()
                val canMapAll = inner.isNotEmpty() && inner.all { subscriptMap.containsKey(it) || it == ' ' || it == ',' }
                if (canMapAll) {
                    inner.map { subscriptMap[it] ?: it.toString() }.joinToString("")
                } else {
                    "₍$inner₎"
                }
            }
            str = subSingleRegex.replace(str) { match ->
                val inner = match.groupValues[1]
                val char = inner.firstOrNull()
                if (char != null && subscriptMap.containsKey(char)) {
                    subscriptMap[char]!!
                } else {
                    "₍$inner₎"
                }
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
            val formula = match.groupValues[1].ifEmpty { match.groupValues[2] }
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

        // 5. Transform standalone LaTeX constructs & symbols in bare prose
        if (protectedText.contains('\\')) {
            protectedText = unwrapTextAndFontCommands(protectedText)
            protectedText = replaceUnderOverBraces(protectedText)
            protectedText = replaceExtensibleArrows(protectedText)
            protectedText = replaceStructuralMathCommands(protectedText)
            protectedText = cleanSizingAndDelimiters(protectedText, isMathMode = false)
            protectedText = replaceSymbolsSinglePass(protectedText)
            protectedText = fontFallbackRegex.replace(protectedText, "$1")
            protectedText = replaceBareOperatorLimits(protectedText)
        }

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
