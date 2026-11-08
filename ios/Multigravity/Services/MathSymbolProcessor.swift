import Foundation

/// Comprehensive LaTeX math and mathematical symbol processor.
/// Converts LaTeX formulas, inline/block math, arrows, operators, Greek letters,
/// superscripts, subscripts, and structural commands into clean, native Unicode typography.
public enum MathSymbolProcessor: Sendable {
    
    // MARK: - Symbol Dictionaries
    
    /// Pre-sorted symbol mappings (longest key first to prevent prefix shadowing)
    private static let symbolReplacements: [(pattern: String, replacement: String)] = [
        // Blackboard bold (\mathbb & \mathbbm)
        (#"\\mathbb\{A\}"#, "𝔸"), (#"\\mathbb\{B\}"#, "𝔹"), (#"\\mathbb\{C\}"#, "ℂ"), (#"\\mathbf\{C\}"#, "ℂ"),
        (#"\\mathbb\{D\}"#, "𝔻"), (#"\\mathbb\{E\}"#, "𝔼"), (#"\\mathbb\{F\}"#, "𝔽"), (#"\\mathbb\{G\}"#, "𝔾"),
        (#"\\mathbb\{H\}"#, "ℍ"), (#"\\mathbb\{I\}"#, "𝕀"), (#"\\mathbb\{J\}"#, "𝕁"), (#"\\mathbb\{K\}"#, "𝕂"),
        (#"\\mathbb\{L\}"#, "𝕃"), (#"\\mathbb\{M\}"#, "𝕄"), (#"\\mathbb\{N\}"#, "ℕ"), (#"\\mathbf\{N\}"#, "ℕ"),
        (#"\\mathbb\{O\}"#, "𝕆"), (#"\\mathbb\{P\}"#, "ℙ"), (#"\\mathbb\{Q\}"#, "ℚ"), (#"\\mathbf\{Q\}"#, "ℚ"),
        (#"\\mathbb\{R\}"#, "ℝ"), (#"\\mathbf\{R\}"#, "ℝ"), (#"\\mathbb\{S\}"#, "𝕊"), (#"\\mathbb\{T\}"#, "𝕋"),
        (#"\\mathbb\{U\}"#, "𝕌"), (#"\\mathbb\{V\}"#, "𝕍"), (#"\\mathbb\{W\}"#, "𝕎"), (#"\\mathbb\{X\}"#, "𝕏"),
        (#"\\mathbb\{Y\}"#, "𝕐"), (#"\\mathbb\{Z\}"#, "ℤ"), (#"\\mathbf\{Z\}"#, "ℤ"),
        (#"\\mathbb\{0\}"#, "𝟘"), (#"\\mathbb\{1\}"#, "𝟙"), (#"\\mathbbm\{1\}"#, "𝟙"), (#"\\mathbb\{k\}"#, "𝕜"),
        
        // Calligraphic (\mathcal) - full A..Z
        (#"\\mathcal\{A\}"#, "𝒜"), (#"\\mathcal\{B\}"#, "ℬ"), (#"\\mathcal\{C\}"#, "𝒞"), (#"\\mathcal\{D\}"#, "𝒟"),
        (#"\\mathcal\{E\}"#, "ℰ"), (#"\\mathcal\{F\}"#, "ℱ"), (#"\\mathcal\{G\}"#, "𝒢"), (#"\\mathcal\{H\}"#, "ℋ"),
        (#"\\mathcal\{I\}"#, "ℐ"), (#"\\mathcal\{J\}"#, "𝒥"), (#"\\mathcal\{K\}"#, "𝒦"), (#"\\mathcal\{L\}"#, "ℒ"),
        (#"\\mathcal\{M\}"#, "ℳ"), (#"\\mathcal\{N\}"#, "𝒩"), (#"\\mathcal\{O\}"#, "𝒪"), (#"\\mathcal\{P\}"#, "𝒫"),
        (#"\\mathcal\{Q\}"#, "𝒬"), (#"\\mathcal\{R\}"#, "ℛ"), (#"\\mathcal\{S\}"#, "𝒮"), (#"\\mathcal\{T\}"#, "𝒯"),
        (#"\\mathcal\{U\}"#, "𝒰"), (#"\\mathcal\{V\}"#, "𝒱"), (#"\\mathcal\{W\}"#, "𝒲"), (#"\\mathcal\{X\}"#, "𝒳"),
        (#"\\mathcal\{Y\}"#, "𝒴"), (#"\\mathcal\{Z\}"#, "𝒵"),
        
        // Script (\mathscr) - full A..Z
        (#"\\mathscr\{A\}"#, "𝒜"), (#"\\mathscr\{B\}"#, "ℬ"), (#"\\mathscr\{C\}"#, "𝒞"), (#"\\mathscr\{D\}"#, "𝒟"),
        (#"\\mathscr\{E\}"#, "ℰ"), (#"\\mathscr\{F\}"#, "ℱ"), (#"\\mathscr\{G\}"#, "𝒢"), (#"\\mathscr\{H\}"#, "ℋ"),
        (#"\\mathscr\{I\}"#, "ℐ"), (#"\\mathscr\{J\}"#, "𝒥"), (#"\\mathscr\{K\}"#, "𝒦"), (#"\\mathscr\{L\}"#, "ℒ"),
        (#"\\mathscr\{M\}"#, "ℳ"), (#"\\mathscr\{N\}"#, "𝒩"), (#"\\mathscr\{O\}"#, "𝒪"), (#"\\mathscr\{P\}"#, "𝒫"),
        (#"\\mathscr\{Q\}"#, "𝒬"), (#"\\mathscr\{R\}"#, "ℛ"), (#"\\mathscr\{S\}"#, "𝒮"), (#"\\mathscr\{T\}"#, "𝒯"),
        (#"\\mathscr\{U\}"#, "𝒰"), (#"\\mathscr\{V\}"#, "𝒱"), (#"\\mathscr\{W\}"#, "𝒲"), (#"\\mathscr\{X\}"#, "𝒳"),
        (#"\\mathscr\{Y\}"#, "𝒴"), (#"\\mathscr\{Z\}"#, "𝒵"),
        
        // Fraktur (\mathfrak) - A..Z, a..z
        (#"\\mathfrak\{A\}"#, "𝔄"), (#"\\mathfrak\{B\}"#, "𝔅"), (#"\\mathfrak\{C\}"#, "ℭ"), (#"\\mathfrak\{D\}"#, "𝔇"),
        (#"\\mathfrak\{E\}"#, "𝔈"), (#"\\mathfrak\{F\}"#, "𝔉"), (#"\\mathfrak\{G\}"#, "𝔊"), (#"\\mathfrak\{H\}"#, "ℌ"),
        (#"\\mathfrak\{I\}"#, "ℑ"), (#"\\mathfrak\{J\}"#, "𝔍"), (#"\\mathfrak\{K\}"#, "𝔎"), (#"\\mathfrak\{L\}"#, "𝔏"),
        (#"\\mathfrak\{M\}"#, "𝔐"), (#"\\mathfrak\{N\}"#, "𝔑"), (#"\\mathfrak\{O\}"#, "𝔒"), (#"\\mathfrak\{P\}"#, "𝔓"),
        (#"\\mathfrak\{Q\}"#, "𝔔"), (#"\\mathfrak\{R\}"#, "ℜ"), (#"\\mathfrak\{S\}"#, "𝔖"), (#"\\mathfrak\{T\}"#, "𝔗"),
        (#"\\mathfrak\{U\}"#, "𝔘"), (#"\\mathfrak\{V\}"#, "𝔙"), (#"\\mathfrak\{W\}"#, "𝔚"), (#"\\mathfrak\{X\}"#, "𝔛"),
        (#"\\mathfrak\{Y\}"#, "𝔜"), (#"\\mathfrak\{Z\}"#, "ℨ"),
        (#"\\mathfrak\{a\}"#, "𝔞"), (#"\\mathfrak\{b\}"#, "𝔟"), (#"\\mathfrak\{c\}"#, "𝔠"), (#"\\mathfrak\{d\}"#, "𝔡"),
        (#"\\mathfrak\{e\}"#, "𝔢"), (#"\\mathfrak\{f\}"#, "𝔣"), (#"\\mathfrak\{g\}"#, "𝔤"), (#"\\mathfrak\{h\}"#, "𝔥"),
        (#"\\mathfrak\{i\}"#, "𝔦"), (#"\\mathfrak\{j\}"#, "𝔧"), (#"\\mathfrak\{k\}"#, "𝔨"), (#"\\mathfrak\{l\}"#, "𝔩"),
        (#"\\mathfrak\{m\}"#, "𝔪"), (#"\\mathfrak\{n\}"#, "𝔫"), (#"\\mathfrak\{o\}"#, "𝔬"), (#"\\mathfrak\{p\}"#, "𝔭"),
        (#"\\mathfrak\{q\}"#, "𝔮"), (#"\\mathfrak\{r\}"#, "𝔯"), (#"\\mathfrak\{s\}"#, "𝔰"), (#"\\mathfrak\{t\}"#, "𝔱"),
        (#"\\mathfrak\{u\}"#, "𝔲"), (#"\\mathfrak\{v\}"#, "𝔳"), (#"\\mathfrak\{w\}"#, "𝔴"), (#"\\mathfrak\{x\}"#, "𝔵"),
        (#"\\mathfrak\{y\}"#, "𝔶"), (#"\\mathfrak\{z\}"#, "𝔷"),
        
        // Delimiters, Brackets & Norms
        (#"\\langle"#, "⟨"),
        (#"\\rangle"#, "⟩"),
        (#"\\lfloor"#, "⌊"),
        (#"\\rfloor"#, "⌋"),
        (#"\\lceil"#, "⌈"),
        (#"\\rceil"#, "⌉"),
        (#"\\ulcorner"#, "⌜"),
        (#"\\urcorner"#, "⌝"),
        (#"\\llcorner"#, "⌞"),
        (#"\\lrcorner"#, "⌟"),
        (#"\\lVert"#, "‖"),
        (#"\\rVert"#, "‖"),
        (#"\\Vert"#, "‖"),
        (#"\\lvert"#, "|"),
        (#"\\rvert"#, "|"),
        (#"\\vert"#, "|"),
        (#"\\mid\b"#, "∣"),
        (#"\\nmid"#, "∤"),
        (#"\\backslash"#, "\\"),
        
        // Long & squiggly arrows
        (#"\\longleftrightarrow"#, "⟷"),
        (#"\\Longleftrightarrow"#, "⟺"),
        (#"\\leftrightsquigarrow"#, "↭"),
        (#"\\rightsquigarrow"#, "⇝"),
        (#"\\twoheadrightarrow"#, "↠"),
        (#"\\twoheadleftarrow"#, "↞"),
        (#"\\longrightarrow"#, "⟶"),
        (#"\\longleftarrow"#, "⟵"),
        (#"\\Longrightarrow"#, "⟹"),
        (#"\\Longleftarrow"#, "⟸"),
        (#"\\rightarrowtail"#, "↣"),
        (#"\\leftarrowtail"#, "↢"),
        (#"\\circlearrowleft"#, "↺"),
        (#"\\circlearrowright"#, "↻"),
        (#"\\curvearrowleft"#, "↶"),
        (#"\\curvearrowright"#, "↷"),
        (#"\\longmapsto"#, "⟼"),
        (#"\\mapsfrom"#, "↤"),
        (#"\\leadsto"#, "↝"),
        
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
        (#"\\impliedby"#, "⇐"),
        (#"\\iff"#, "⇔"),
        (#"\\to\b"#, "→"),
        (#"\\gets\b"#, "←"),
        
        // Comparisons & Relations
        (#"\\fallingdotseq"#, "≒"),
        (#"\\risingdotseq"#, "≓"),
        (#"\\lessapprox"#, "⪅"),
        (#"\\gtrapprox"#, "⪆"),
        (#"\\sqsubseteq"#, "⊑"),
        (#"\\sqsupseteq"#, "⊒"),
        (#"\\sqsubset"#, "⊏"),
        (#"\\sqsupset"#, "⊐"),
        (#"\\triangleq"#, "≜"),
        (#"\\coloneqq"#, "≔"),
        (#"\\eqqcolon"#, "≕"),
        (#"\\coloneq"#, "≔"),
        (#"\\eqcolon"#, "≕"),
        (#"\\defeq"#, "≔"),
        (#"\\leqslant"#, "≤"),
        (#"\\geqslant"#, "≥"),
        (#"\\subseteq"#, "⊆"),
        (#"\\supseteq"#, "⊇"),
        (#"\\subsetneq"#, "⊊"),
        (#"\\supsetneq"#, "⊋"),
        (#"\\nsubseteq"#, "⊈"),
        (#"\\nsupseteq"#, "⊉"),
        (#"\\nsubset"#, "⊄"),
        (#"\\parallel"#, "∥"),
        (#"\\nparallel"#, "∦"),
        (#"\\lesssim"#, "≲"),
        (#"\\gtrsim"#, "≳"),
        (#"\\napprox"#, "≉"),
        (#"\\nequiv"#, "≢"),
        (#"\\circeq"#, "≗"),
        (#"\\preceq"#, "⪯"),
        (#"\\succeq"#, "⪰"),
        (#"\\approx"#, "≈"),
        (#"\\simeq"#, "≃"),
        (#"\\ncong"#, "≇"),
        (#"\\cong"#, "≅"),
        (#"\\equiv"#, "≡"),
        (#"\\propto"#, "∝"),
        (#"\\asymp"#, "≍"),
        (#"\\doteq"#, "≐"),
        (#"\\bowtie"#, "⋈"),
        (#"\\ltimes"#, "⋉"),
        (#"\\rtimes"#, "⋊"),
        (#"\\prec"#, "≺"),
        (#"\\succ"#, "≻"),
        (#"\\nsim"#, "≁"),
        (#"\\sim\b"#, "∼"),
        (#"\\perp"#, "⊥"),
        (#"\\ll\b"#, "≪"),
        (#"\\gg\b"#, "≫"),
        (#"\\leq"#, "≤"),
        (#"\\geq"#, "≥"),
        (#"\\neq"#, "≠"),
        (#"\\le\b"#, "≤"),
        (#"\\ge\b"#, "≥"),
        (#"\\ne\b"#, "≠"),
        
        // Set & Logic
        (#"\\smallsetminus"#, "∖"),
        (#"\\complement"#, "∁"),
        (#"\\varnothing"#, "∅"),
        (#"\\emptyset"#, "∅"),
        (#"\\empty\b"#, "∅"),
        (#"\\setminus"#, "∖"),
        (#"\\therefore"#, "∴"),
        (#"\\because"#, "∵"),
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
        (#"\\vee\b"#, "∨"),
        (#"\\top\b"#, "⊤"),
        (#"\\bot\b"#, "⊥"),
        (#"\\vdash"#, "⊢"),
        (#"\\dashv"#, "⊣"),
        (#"\\vDash"#, "⊨"),
        (#"\\Vdash"#, "⊩"),
        (#"\\models"#, "⊨"),
        (#"\\bigcup"#, "⋃"),
        (#"\\bigcap"#, "⋂"),
        (#"\\bigsqcup"#, "⨆"),
        (#"\\biguplus"#, "⨄"),
        (#"\\bigvee"#, "⋁"),
        (#"\\bigwedge"#, "⋀"),
        (#"\\sqcup"#, "⊔"),
        (#"\\sqcap"#, "⊓"),
        (#"\\uplus"#, "⊎"),
        (#"\\cup\b"#, "∪"),
        (#"\\cap\b"#, "∩"),
        (#"\\in\b"#, "∈"),
        (#"\\ni\b"#, "∋"),
        (#"\\owns"#, "∋"),
        (#"\\neg\b"#, "¬"),
        
        // Arithmetic & Operators
        (#"\\bigoplus"#, "⨁"),
        (#"\\bigotimes"#, "⨂"),
        (#"\\bigodot"#, "⨀"),
        (#"\\boxtimes"#, "⊠"),
        (#"\\boxplus"#, "⊞"),
        (#"\\boxminus"#, "⊟"),
        (#"\\boxdot"#, "⊡"),
        (#"\\times"#, "×"),
        (#"\\div\b"#, "÷"),
        (#"\\pm\b"#, "±"),
        (#"\\mp\b"#, "∓"),
        (#"\\cdot"#, "·"),
        (#"\\cdots"#, "⋯"),
        (#"\\ldots"#, "…"),
        (#"\\dotsc"#, "…"),
        (#"\\dotsb"#, "⋯"),
        (#"\\dotsm"#, "⋯"),
        (#"\\dotsi"#, "⋯"),
        (#"\\dots\b"#, "…"),
        (#"\\ddots"#, "⋱"),
        (#"\\vdots"#, "⋮"),
        (#"\\bullet"#, "•"),
        (#"\\circ\b"#, "∘"),
        (#"\\star"#, "⋆"),
        (#"\\ast\b"#, "∗"),
        (#"\\oplus"#, "⊕"),
        (#"\\ominus"#, "⊖"),
        (#"\\otimes"#, "⊗"),
        (#"\\oslash"#, "⊘"),
        (#"\\odot"#, "⊙"),
        (#"\\dagger"#, "†"),
        (#"\\ddagger"#, "‡"),
        (#"\\wr\b"#, "≀"),
        (#"\\colon"#, ":"),
        (#"\\prime"#, "′"),
        (#"\\checkmark"#, "✓"),
        
        // Calculus, Analysis & Geometry
        (#"\\oiiint"#, "∰"),
        (#"\\oiint"#, "∯"),
        (#"\\iiint"#, "∭"),
        (#"\\iint"#, "∬"),
        (#"\\oint"#, "∮"),
        (#"\\int\b"#, "∫"),
        (#"\\sum\b"#, "∑"),
        (#"\\prod\b"#, "∏"),
        (#"\\coprod"#, "∐"),
        (#"\\partial"#, "∂"),
        (#"\\nabla"#, "∇"),
        (#"\\infty"#, "∞"),
        (#"\\surd"#, "√"),
        (#"\\sqrt"#, "√"),
        (#"\\angle"#, "∠"),
        (#"\\measuredangle"#, "∡"),
        (#"\\sphericalangle"#, "∢"),
        (#"\\blacktriangledown"#, "▼"),
        (#"\\bigtriangledown"#, "▽"),
        (#"\\blacktriangle"#, "▲"),
        (#"\\bigtriangleup"#, "△"),
        (#"\\triangleleft"#, "◁"),
        (#"\\triangleright"#, "▷"),
        (#"\\triangle"#, "△"),
        (#"\\blacksquare"#, "■"),
        (#"\\diamond"#, "◇"),
        (#"\\Diamond"#, "◇"),
        (#"\\lozenge"#, "◊"),
        (#"\\square"#, "□"),
        (#"\\Box\b"#, "□"),
        (#"\\degree"#, "°"),
        (#"\\hbar"#, "ℏ"),
        (#"\\ell\b"#, "ℓ"),
        (#"\\aleph"#, "ℵ"),
        (#"\\beth"#, "ℶ"),
        (#"\\gimel"#, "ℷ"),
        (#"\\daleth"#, "ℸ"),
        (#"\\wp\b"#, "℘"),
        (#"\\Re\b"#, "ℜ"),
        (#"\\Im\b"#, "ℑ"),
        (#"\\mho"#, "℧"),
        
        // Standard Math Functions
        (#"\\limsup"#, "lim sup"),
        (#"\\liminf"#, "lim inf"),
        (#"\\argmax"#, "arg max"),
        (#"\\argmin"#, "arg min"),
        (#"\\arcsin"#, "arcsin"),
        (#"\\arccos"#, "arccos"),
        (#"\\arctan"#, "arctan"),
        (#"\\sinh"#, "sinh"),
        (#"\\cosh"#, "cosh"),
        (#"\\tanh"#, "tanh"),
        (#"\\coth"#, "coth"),
        (#"\\lim\b"#, "lim"),
        (#"\\sup\b"#, "sup"),
        (#"\\inf\b"#, "inf"),
        (#"\\max\b"#, "max"),
        (#"\\min\b"#, "min"),
        (#"\\arg\b"#, "arg"),
        (#"\\det\b"#, "det"),
        (#"\\dim\b"#, "dim"),
        (#"\\ker\b"#, "ker"),
        (#"\\hom\b"#, "hom"),
        (#"\\deg\b"#, "deg"),
        (#"\\gcd\b"#, "gcd"),
        (#"\\lcm\b"#, "lcm"),
        (#"\\exp\b"#, "exp"),
        (#"\\log\b"#, "log"),
        (#"\\ln\b"#, "ln"),
        (#"\\lg\b"#, "lg"),
        (#"\\sin\b"#, "sin"),
        (#"\\cos\b"#, "cos"),
        (#"\\tan\b"#, "tan"),
        (#"\\cot\b"#, "cot"),
        (#"\\sec\b"#, "sec"),
        (#"\\csc\b"#, "csc"),
        (#"\\Pr\b"#, "Pr"),
        (#"\\bmod"#, "mod"),
        
        // Layout / Spacing / Style No-ops
        (#"\\displaystyle"#, ""),
        (#"\\scriptscriptstyle"#, ""),
        (#"\\scriptstyle"#, ""),
        (#"\\textstyle"#, ""),
        (#"\\nolimits"#, ""),
        (#"\\limits"#, ""),
        (#"\\noindent"#, ""),
        (#"\\qquad"#, "  "),
        (#"\\quad"#, " "),
        (#"\\enspace"#, " "),
        (#"\\thinspace"#, " "),
        
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
        (#"\\omicron"#, "ο"),
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
        "A": "ᴬ", "B": "ᴮ", "D": "ᴰ", "E": "ᴱ", "G": "ᴳ",
        "H": "ᴴ", "I": "ᴵ", "J": "ᴶ", "K": "ᴷ", "L": "ᴸ",
        "M": "ᴹ", "N": "ᴺ", "O": "ᴼ", "P": "ᴾ", "R": "ᴿ",
        "T": "ᵀ", "U": "ᵁ", "V": "ⱽ", "W": "ᵂ",
        "⊤": "ᵀ", "∘": "°", "′": "′", "*": "*", "∗": "*"
    ]
    
    private static let subscriptDict: [Character: Character] = [
        "0": "₀", "1": "₁", "2": "₂", "3": "₃", "4": "₄",
        "5": "₅", "6": "₆", "7": "₇", "8": "₈", "9": "₉",
        "+": "₊", "-": "₋", "=": "₌", "(": "₍", ")": "₎",
        "a": "ₐ", "e": "ₑ", "h": "ₕ", "i": "ᵢ", "j": "ⱼ",
        "k": "ₖ", "l": "ₗ", "m": "ₘ", "n": "ₙ", "o": "ₒ",
        "p": "ₚ", "r": "ᵣ", "s": "ₛ", "t": "ₜ", "u": "ᵤ",
        "v": "ᵥ", "x": "ₓ",
        "β": "ᵦ", "γ": "ᵧ", "ρ": "ᵨ", "φ": "ᵩ", "χ": "ᵪ"
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
    
    /// Dictionary mapping LaTeX command names (without leading backslash) to Unicode replacements.
    /// Built once from symbolReplacements for O(1) lookup during single-pass scanning.
    private static let symbolLookup: [String: String] = {
        var dict: [String: String] = [:]
        for (pattern, replacement) in symbolReplacements {
            var cmd = pattern
            while cmd.hasPrefix("\\") {
                cmd = String(cmd.dropFirst())
            }
            if cmd.hasSuffix(#"\b"#) {
                cmd = String(cmd.dropLast(2))
            }
            cmd = cmd.replacingOccurrences(of: #"\\{"#, with: "{")
                     .replacingOccurrences(of: #"\{"#, with: "{")
                     .replacingOccurrences(of: #"\\}"#, with: "}")
                     .replacingOccurrences(of: #"\}"#, with: "}")
            dict[cmd] = replacement
        }
        return dict
    }()
    
    /// Commands that require word-boundary checks
    private static let wordBoundaryCommands: Set<String> = [
        "to", "gets", "le", "ge", "ne", "in", "ni", "empty", "sim",
        "mid", "ll", "gg", "cup", "cap", "neg", "top", "bot", "vee",
        "div", "pm", "mp", "ast", "wr", "int", "sum", "prod", "ell",
        "wp", "Re", "Im", "Box", "lim", "sup", "inf", "max", "min",
        "arg", "det", "dim", "ker", "hom", "deg", "gcd", "lcm", "exp",
        "log", "ln", "lg", "sin", "cos", "tan", "cot", "sec", "csc", "Pr"
    ]
    
    /// Performs a single-pass scan replacing LaTeX \commands with Unicode symbols.
    private static func replaceSymbolsSinglePass(_ input: String) -> String {
        guard input.contains("\\") else { return input }
        
        var result = ""
        result.reserveCapacity(input.count)
        let chars = Array(input)
        var i = 0
        
        while i < chars.count {
            if chars[i] == "\\" && i + 1 < chars.count && chars[i + 1].isASCII && chars[i + 1].isLetter {
                var j = i + 1
                while j < chars.count && chars[j].isASCII && chars[j].isLetter { j += 1 }
                let cmdName = String(chars[(i + 1)..<j])
                
                var fullKey = cmdName
                if j < chars.count && chars[j] == "{" {
                    if let closeBrace = chars[(j+1)...].firstIndex(of: "}") {
                        let paramKey = cmdName + String(chars[j...closeBrace])
                        if symbolLookup[paramKey] != nil {
                            fullKey = paramKey
                            j = closeBrace + 1
                        }
                    }
                }
                
                if let replacement = symbolLookup[fullKey] {
                    if wordBoundaryCommands.contains(fullKey) && j < chars.count && chars[j].isASCII && chars[j].isLetter {
                        result.append(chars[i])
                        i += 1
                    } else {
                        // Consume single trailing space after parameterless LaTeX command if followed by word/symbol
                        result.append(contentsOf: replacement)
                        i = j
                    }
                } else {
                    result.append(chars[i])
                    i += 1
                }
            } else {
                result.append(chars[i])
                i += 1
            }
        }
        return result
    }
    
    private static let blockRegex = try? NSRegularExpression(pattern: #"```[a-zA-Z0-9_\-]*\n[\s\S]*?```"#)
    private static let inlineRegex = try? NSRegularExpression(pattern: #"`[^`\n]+`"#)
    private static let displayBlockRegex = try? NSRegularExpression(pattern: #"\$\$(.*?)\$\$|\\\[(.*?)\\\]"#, options: [.dotMatchesLineSeparators])
    private static let inlineMathRegex = try? NSRegularExpression(pattern: #"(?<!\\)\$(?!\s)([^$\n]+?)(?<!\s)(?<!\\)\$|\\\((.*?)\\\)"#)
    
    // Preserves grouping braces when \text{...} is directly attached to _ or ^ without outer braces (e.g., _\text{主语} -> _{主语})
    private static let subSupTextWrapperRegex = try? NSRegularExpression(
        pattern: #"([_^])\\(?:text|textbf|textit|textrm|textsf|texttt|textsc|mathrm|mathbf|mathit|mathsf|mathtt|mathbbm|operatorname\*?|pmb|boldsymbol|bm|bold|emph|mbox|hbox)\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}"#
    )
    private static let textWrapperRegex = try? NSRegularExpression(
        pattern: #"\\(?:text|textbf|textit|textrm|textsf|texttt|textsc|mathrm|mathbf|mathit|mathsf|mathtt|mathbbm|operatorname\*?|pmb|boldsymbol|bm|bold|emph|mbox|hbox)\{([^{}]*)\}"#
    )
    private static let fontFallbackRegex = try? NSRegularExpression(
        pattern: #"\\(?:mathcal|mathscr|mathbb|mathfrak|mathbbm)\{([^{}]*)\}"#
    )
    private static let colorWrapperRegex = try? NSRegularExpression(
        pattern: #"\\(?:textcolor|color)\{[^{}]*\}\{([^{}]*)\}|\\color\{[^{}]*\}"#
    )
    private static let cancelRegex = try? NSRegularExpression(
        pattern: #"\\(?:cancel|bcancel|xcancel|sout)\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}"#
    )
    private static let pmodRegex = try? NSRegularExpression(
        pattern: #"\\pmod\{([^{}]*)\}"#
    )
    private static let binomRegex = try? NSRegularExpression(
        pattern: #"\\(?:d|t)?binom\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}"#
    )
    private static let underbraceRegex = try? NSRegularExpression(
        pattern: #"\\underbrace\s*\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}\s*(?:_\s*\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}|_\s*([a-zA-Z0-9])|\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\})?"#
    )
    private static let overbraceRegex = try? NSRegularExpression(
        pattern: #"\\overbrace\s*\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}\s*(?:\^\s*\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}|\^\s*([a-zA-Z0-9])|\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\})?"#
    )
    private static let fracRegex = try? NSRegularExpression(
        pattern: #"\\(?:d|t|c)?frac\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}"#
    )
    private static let sqrtNRegex = try? NSRegularExpression(
        pattern: #"\\sqrt\[([^\]]*)\]\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}"#
    )
    private static let sqrtRegex = try? NSRegularExpression(
        pattern: #"\\sqrt\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}"#
    )
    private static let accentRegex = try? NSRegularExpression(
        pattern: #"\\(widehat|hat|widetilde|tilde|overline|bar|overrightarrow|vec|ddot|dot|underline)\{([^{}]*)\}"#
    )
    private static let sizingPrefixRegex = try? NSRegularExpression(
        pattern: #"\\(?:left|right|middle|bigl|bigr|Bigl|Bigr|biggl|biggr|Biggl|Biggr|big|Big|bigg|Bigg)\s*(?=[(\[\{|⟨⟩⌊⌋⌈⌉‖/\\])"#
    )
    private static let supGroupRegex = try? NSRegularExpression(pattern: #"\^\{([^{}]+)\}"#)
    private static let supSingleRegex = try? NSRegularExpression(pattern: #"\^([0-9a-zA-Z\+\-\*⊤∘′∗])"#)
    private static let subGroupRegex = try? NSRegularExpression(pattern: #"_\{([^{}]+)\}"#)
    private static let subSingleRegex = try? NSRegularExpression(pattern: #"_([0-9a-zA-Zβγρφχ])"#)
    private static let extensibleArrowRegex = try? NSRegularExpression(
        pattern: #"\\x(rightarrow|leftarrow|Rightarrow|Leftarrow|leftrightarrow|Leftrightarrow|mapsto)(?:\[([^\]]*)\])?\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}"#
    )
    private static let oversetRegex = try? NSRegularExpression(
        pattern: #"\\overset\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}"#
    )
    private static let undersetRegex = try? NSRegularExpression(
        pattern: #"\\underset\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}"#
    )
    private static let boxedRegex = try? NSRegularExpression(
        pattern: #"\\boxed\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}"#
    )
    private static let mathEnvRegex = try? NSRegularExpression(
        pattern: #"\\(?:begin|end)\{(?:aligned|align\*?|matrix|bmatrix|pmatrix|vmatrix|Vmatrix|Bmatrix|cases|split|gather\*?| multline\*?|array|equation\*?|subequations)\}(?:\{[^{}]*\})?"#
    )
    private static let mathTagRegex = try? NSRegularExpression(pattern: #"\\tag\*?\{([^{}]*)\}"#)
    private static let mathLabelRegex = try? NSRegularExpression(pattern: #"\\label\{([^{}]*)\}"#)
    private static let negatedOpRegex = try? NSRegularExpression(
        pattern: #"\\not\s*(\\(?:in|equiv|sim|approx|cong|subset|subseteq|supset|supseteq|parallel|mid)\b|=)"#
    )
    private static let bareOperatorLimitsRegex = try? NSRegularExpression(
        pattern: #"(\b(?:lim sup|lim inf|arg max|arg min|lim|max|min|sup|inf)|[∑∏∐∫∬∭∮∯∰⋃⋂⨁⨂⨀⨆⨄⋁⋀])\s*(?:_\{([^{}]+)\}|_([0-9a-zA-Z]))?(?:\s*(?:\^\{([^{}]+)\}|\^([0-9a-zA-Z\+\-\*⊤∘′∗])))?"#
    )
    
    // MARK: - Main Processing Entry Point
    
    /// Processes Markdown text to replace all LaTeX math symbols with native Unicode characters,
    /// safely preserving fenced code blocks and inline code spans.
    public static func process(_ text: String) -> String {
        guard !text.isEmpty else { return text }
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
                var content = matchText
                if content.hasPrefix("$$") && content.hasSuffix("$$") && content.count >= 4 {
                    content = String(content.dropFirst(2).dropLast(2))
                } else if content.hasPrefix(#"\["#) && content.hasSuffix(#"\]"#) && content.count >= 4 {
                    content = String(content.dropFirst(2).dropLast(2))
                }
                content = content.trimmingCharacters(in: .whitespacesAndNewlines)
                content = content.replacingOccurrences(of: #"\\\\"#, with: "\n")
                return cleanMathExpression(content)
            }
        }
        
        // 4. Process inline math: $...$ and \(...\)
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
        
        // 5. Transform standalone LaTeX constructs & symbol commands in bare prose
        // CRITICAL: Never run bare _ or ^ subscript/superscript replacements on prose outside math blocks!
        if protectedText.contains("\\") {
            protectedText = unwrapTextAndFontCommands(protectedText)
            protectedText = replaceUnderOverBraces(protectedText)
            protectedText = replaceExtensibleArrows(protectedText)
            protectedText = replaceStructuralMathCommands(protectedText)
            protectedText = cleanSizingAndDelimiters(protectedText, isMathMode: false)
            protectedText = replaceSymbolsSinglePass(protectedText)
            if let fontFallbackRegex = Self.fontFallbackRegex {
                protectedText = fontFallbackRegex.stringByReplacingMatches(
                    in: protectedText,
                    range: NSRange(location: 0, length: (protectedText as NSString).length),
                    withTemplate: "$1"
                )
            }
            protectedText = replaceBareOperatorLimits(protectedText)
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
    
    // MARK: - Shared Construct Processors
    
    /// Unwraps \text{...}, \mathrm{...}, \mathbf{...}, \textcolor{...}{...} iteratively while preserving _\text{...} braces.
    private static func unwrapTextAndFontCommands(_ input: String) -> String {
        guard input.contains("\\") else { return input }
        var str = input
        if let subSupTextWrapperRegex = Self.subSupTextWrapperRegex {
            str = subSupTextWrapperRegex.stringByReplacingMatches(
                in: str,
                range: NSRange(location: 0, length: (str as NSString).length),
                withTemplate: "$1{$2}"
            )
        }
        if let colorWrapperRegex = Self.colorWrapperRegex {
            str = colorWrapperRegex.stringByReplacingMatches(
                in: str,
                range: NSRange(location: 0, length: (str as NSString).length),
                withTemplate: "$1"
            )
        }
        if let textWrapperRegex = Self.textWrapperRegex {
            for _ in 0..<4 {
                let nsLen = (str as NSString).length
                let next = textWrapperRegex.stringByReplacingMatches(in: str, range: NSRange(location: 0, length: nsLen), withTemplate: "$1")
                if next == str { break }
                str = next
            }
        }
        return str
    }
    
    /// Converts \underbrace{A}_{B}, \underbrace{A}{B}, \underbrace{A}_B, \underbrace{A} and \overbrace equivalents into A〔B〕.
    public static func replaceUnderOverBraces(_ input: String) -> String {
        guard input.contains("brace") else { return input }
        var str = input
        
        func applyBraceRegex(_ regex: NSRegularExpression?) {
            guard let regex = regex else { return }
            for _ in 0..<4 {
                let nsStr = str as NSString
                let matches = regex.matches(in: str, range: NSRange(location: 0, length: nsStr.length))
                guard !matches.isEmpty else { break }
                for match in matches.reversed() {
                    let base = nsStr.substring(with: match.range(at: 1))
                        .trimmingCharacters(in: .whitespacesAndNewlines)
                        .replacingOccurrences(of: "[ \\t]+", with: " ", options: .regularExpression)
                    var note = ""
                    for groupIdx in [2, 3, 4] {
                        if match.numberOfRanges > groupIdx {
                            let r = match.range(at: groupIdx)
                            if r.location != NSNotFound {
                                note = nsStr.substring(with: r)
                                    .trimmingCharacters(in: .whitespacesAndNewlines)
                                    .replacingOccurrences(of: "[ \\t]+", with: " ", options: .regularExpression)
                                break
                            }
                        }
                    }
                    let replacement = note.isEmpty ? base : "\(base)〔\(note)〕"
                    str = (str as NSString).replacingCharacters(in: match.range, with: replacement)
                }
            }
        }
        
        applyBraceRegex(underbraceRegex)
        applyBraceRegex(overbraceRegex)
        return str
    }
    
    /// Converts subscript/superscript limits directly attached to math operators (lim, max, min, ∑, ∏, ∫, etc.) in bare prose.
    private static func replaceBareOperatorLimits(_ input: String) -> String {
        guard (input.contains("_") || input.contains("^")), let regex = bareOperatorLimitsRegex else { return input }
        let nsStr = input as NSString
        let matches = regex.matches(in: input, range: NSRange(location: 0, length: nsStr.length))
        guard !matches.isEmpty else { return input }
        var str = input
        for match in matches.reversed() {
            let subGroupRange = match.range(at: 2)
            let subSingleRange = match.range(at: 3)
            let supGroupRange = match.range(at: 4)
            let supSingleRange = match.range(at: 5)
            guard subGroupRange.location != NSNotFound || subSingleRange.location != NSNotFound || supGroupRange.location != NSNotFound || supSingleRange.location != NSNotFound else {
                continue
            }
            let op = nsStr.substring(with: match.range(at: 1))
            var subPart = ""
            if subGroupRange.location != NSNotFound {
                let inner = nsStr.substring(with: subGroupRange).trimmingCharacters(in: .whitespaces)
                let canMapAll = !inner.isEmpty && inner.allSatisfy { subscriptDict[$0] != nil || $0 == " " || $0 == "," }
                subPart = canMapAll ? String(inner.map { subscriptDict[$0] ?? $0 }) : "₍\(inner)₎"
            } else if subSingleRange.location != NSNotFound {
                let inner = nsStr.substring(with: subSingleRange)
                if let first = inner.first, let sub = subscriptDict[first] {
                    subPart = String(sub)
                } else {
                    subPart = "₍\(inner)₎"
                }
            }
            var supPart = ""
            if supGroupRange.location != NSNotFound {
                let inner = nsStr.substring(with: supGroupRange).trimmingCharacters(in: .whitespaces)
                let canMapAll = !inner.isEmpty && inner.allSatisfy { superscriptDict[$0] != nil || $0 == " " || $0 == "," }
                supPart = canMapAll ? String(inner.map { superscriptDict[$0] ?? $0 }) : "⁽\(inner)⁾"
            } else if supSingleRange.location != NSNotFound {
                let inner = nsStr.substring(with: supSingleRange)
                if let first = inner.first, let sup = superscriptDict[first] {
                    supPart = String(sup)
                } else {
                    supPart = "⁽\(inner)⁾"
                }
            }
            str = (str as NSString).replacingCharacters(in: match.range, with: "\(op)\(subPart)\(supPart)")
        }
        return str
    }
    
    /// Replaces \boxed, \overset, \underset, \binom, \frac, \sqrt, \cancel, \pmod, and math accents.
    private static func replaceStructuralMathCommands(_ input: String) -> String {
        guard input.contains("\\") else { return input }
        var str = input
        
        // Negated operators: \not\in -> ∉, \not= -> ≠, \not\equiv -> ≢, etc.
        if str.contains(#"\not"#), let negatedOpRegex = Self.negatedOpRegex {
            let nsStr = str as NSString
            let matches = negatedOpRegex.matches(in: str, range: NSRange(location: 0, length: nsStr.length))
            for match in matches.reversed() {
                let op = nsStr.substring(with: match.range(at: 1))
                let rep: String
                switch op {
                case #"\in"#: rep = "∉"
                case "=": rep = "≠"
                case #"\equiv"#: rep = "≢"
                case #"\sim"#: rep = "≁"
                case #"\approx"#: rep = "≉"
                case #"\cong"#: rep = "≇"
                case #"\subset"#: rep = "⊄"
                case #"\subseteq"#: rep = "⊈"
                case #"\supset"#: rep = "⊅"
                case #"\supseteq"#: rep = "⊉"
                case #"\parallel"#: rep = "∦"
                case #"\mid"#: rep = "∤"
                default: rep = op
                }
                str = (str as NSString).replacingCharacters(in: match.range, with: rep)
            }
        }
        
        if let boxedRegex = Self.boxedRegex {
            str = boxedRegex.stringByReplacingMatches(in: str, range: NSRange(location: 0, length: (str as NSString).length), withTemplate: "[$1]")
        }
        if let cancelRegex = Self.cancelRegex {
            str = cancelRegex.stringByReplacingMatches(in: str, range: NSRange(location: 0, length: (str as NSString).length), withTemplate: "~~$1~~")
        }
        if let pmodRegex = Self.pmodRegex {
            str = pmodRegex.stringByReplacingMatches(in: str, range: NSRange(location: 0, length: (str as NSString).length), withTemplate: "(mod $1)")
        }
        if let oversetRegex = Self.oversetRegex {
            str = oversetRegex.stringByReplacingMatches(in: str, range: NSRange(location: 0, length: (str as NSString).length), withTemplate: "$2 ($1)")
        }
        if let undersetRegex = Self.undersetRegex {
            str = undersetRegex.stringByReplacingMatches(in: str, range: NSRange(location: 0, length: (str as NSString).length), withTemplate: "$2 ($1)")
        }
        if let binomRegex = Self.binomRegex {
            str = binomRegex.stringByReplacingMatches(in: str, range: NSRange(location: 0, length: (str as NSString).length), withTemplate: "C($1, $2)")
        }
        if let sqrtNRegex = Self.sqrtNRegex {
            let nsStr = str as NSString
            let matches = sqrtNRegex.matches(in: str, range: NSRange(location: 0, length: nsStr.length))
            for match in matches.reversed() {
                let nRaw = nsStr.substring(with: match.range(at: 1))
                let nSup = String(nRaw.map { superscriptDict[$0] ?? $0 })
                let rad = nsStr.substring(with: match.range(at: 2))
                str = (str as NSString).replacingCharacters(in: match.range, with: "\(nSup)√(\(rad))")
            }
        }
        if let sqrtRegex = Self.sqrtRegex {
            for _ in 0..<2 {
                let next = sqrtRegex.stringByReplacingMatches(in: str, range: NSRange(location: 0, length: (str as NSString).length), withTemplate: "√($1)")
                if next == str { break }
                str = next
            }
        }
        if let fracRegex = Self.fracRegex {
            for _ in 0..<3 {
                let next = fracRegex.stringByReplacingMatches(in: str, range: NSRange(location: 0, length: (str as NSString).length), withTemplate: "$1 / $2")
                if next == str { break }
                str = next
            }
        }
        if let accentRegex = Self.accentRegex {
            let nsStr = str as NSString
            let matches = accentRegex.matches(in: str, range: NSRange(location: 0, length: nsStr.length))
            for match in matches.reversed() {
                let cmd = nsStr.substring(with: match.range(at: 1))
                let base = nsStr.substring(with: match.range(at: 2)).trimmingCharacters(in: .whitespaces)
                let combining: String = {
                    switch cmd {
                    case "hat", "widehat": return "\u{0302}"
                    case "tilde", "widetilde": return "\u{0303}"
                    case "bar", "overline": return "\u{0304}"
                    case "vec", "overrightarrow": return "\u{20D7}"
                    case "dot": return "\u{0307}"
                    case "ddot": return "\u{0308}"
                    case "underline": return "\u{0332}"
                    default: return ""
                    }
                }()
                let rep: String
                if base.isEmpty {
                    rep = ""
                } else if base.count == 1 || cmd == "overline" || cmd == "underline" {
                    rep = base.map { "\($0)\(combining)" }.joined()
                } else {
                    rep = base + combining
                }
                str = (str as NSString).replacingCharacters(in: match.range, with: rep)
            }
        }
        return str
    }
    
    private static func cleanSizingAndDelimiters(_ input: String, isMathMode: Bool) -> String {
        var str = input
            .replacingOccurrences(of: #"\left."#, with: "")
            .replacingOccurrences(of: #"\right."#, with: "")
            .replacingOccurrences(of: #"\left\|"#, with: "‖")
            .replacingOccurrences(of: #"\right\|"#, with: "‖")
            .replacingOccurrences(of: #"\left|"#, with: "|")
            .replacingOccurrences(of: #"\right|"#, with: "|")
            .replacingOccurrences(of: #"\left("#, with: "(")
            .replacingOccurrences(of: #"\right)"#, with: ")")
            .replacingOccurrences(of: #"\left["#, with: "[")
            .replacingOccurrences(of: #"\right]"#, with: "]")
            .replacingOccurrences(of: #"\left\{"#, with: "{")
            .replacingOccurrences(of: #"\right\}"#, with: "}")
        if let sizingPrefixRegex = Self.sizingPrefixRegex {
            str = sizingPrefixRegex.stringByReplacingMatches(
                in: str,
                range: NSRange(location: 0, length: (str as NSString).length),
                withTemplate: ""
            )
        }
        if isMathMode {
            str = str
                .replacingOccurrences(of: #"\|"#, with: "‖")
                .replacingOccurrences(of: #"\{"#, with: "{")
                .replacingOccurrences(of: #"\}"#, with: "}")
                .replacingOccurrences(of: #"\%"#, with: "%")
                .replacingOccurrences(of: #"\_"#, with: "_")
                .replacingOccurrences(of: #"\,"#, with: " ")
                .replacingOccurrences(of: #"\;"#, with: " ")
                .replacingOccurrences(of: #"\:"# , with: " ")
                .replacingOccurrences(of: #"\!"#, with: "")
                .replacingOccurrences(of: #"\quad"#, with: " ")
                .replacingOccurrences(of: #"\qquad"#, with: "  ")
        }
        return str
    }
    
    // MARK: - Extensible Arrows Processor
    
    /// Translates dynamic extensible arrows like \xrightarrow{text} into clean native Unicode typography.
    public static func replaceExtensibleArrows(_ input: String) -> String {
        guard input.contains(#"\x"#), let regex = extensibleArrowRegex else { return input }
        let nsText = input as NSString
        let matches = regex.matches(in: input, range: NSRange(location: 0, length: nsText.length))
        guard !matches.isEmpty else { return input }
        
        var result = input
        for match in matches.reversed() {
            let arrowType = nsText.substring(with: match.range(at: 1))
            let subRange = match.range(at: 2)
            let rawSub = subRange.location != NSNotFound ? nsText.substring(with: subRange) : nil
            let rawSup = nsText.substring(with: match.range(at: 3))
            
            let sup = cleanArrowLabel(rawSup)
            let sub = rawSub != nil ? cleanArrowLabel(rawSub!) : ""
            
            let replacement: String
            switch arrowType {
            case "rightarrow":
                if !sup.isEmpty && !sub.isEmpty {
                    replacement = "──(\(sup) / \(sub))──>"
                } else if !sup.isEmpty {
                    replacement = "──(\(sup))──>"
                } else if !sub.isEmpty {
                    replacement = "──(\(sub))──>"
                } else {
                    replacement = "⟶"
                }
            case "leftarrow":
                if !sup.isEmpty && !sub.isEmpty {
                    replacement = "<──(\(sup) / \(sub))──"
                } else if !sup.isEmpty {
                    replacement = "<──(\(sup))──"
                } else if !sub.isEmpty {
                    replacement = "<──(\(sub))──"
                } else {
                    replacement = "⟵"
                }
            case "Rightarrow":
                if !sup.isEmpty && !sub.isEmpty {
                    replacement = "══(\(sup) / \(sub))══>"
                } else if !sup.isEmpty {
                    replacement = "══(\(sup))══>"
                } else if !sub.isEmpty {
                    replacement = "══(\(sub))══>"
                } else {
                    replacement = "⟹"
                }
            case "Leftarrow":
                if !sup.isEmpty && !sub.isEmpty {
                    replacement = "<══(\(sup) / \(sub))══"
                } else if !sup.isEmpty {
                    replacement = "<══(\(sup))══"
                } else if !sub.isEmpty {
                    replacement = "<══(\(sub))══"
                } else {
                    replacement = "⟸"
                }
            case "leftrightarrow":
                if !sup.isEmpty && !sub.isEmpty {
                    replacement = "<──(\(sup) / \(sub))──>"
                } else if !sup.isEmpty {
                    replacement = "<──(\(sup))──>"
                } else if !sub.isEmpty {
                    replacement = "<──(\(sub))──>"
                } else {
                    replacement = "⟷"
                }
            case "Leftrightarrow":
                if !sup.isEmpty && !sub.isEmpty {
                    replacement = "<══(\(sup) / \(sub))══>"
                } else if !sup.isEmpty {
                    replacement = "<══(\(sup))══>"
                } else if !sub.isEmpty {
                    replacement = "<══(\(sub))══>"
                } else {
                    replacement = "⟺"
                }
            case "mapsto":
                if !sup.isEmpty && !sub.isEmpty {
                    replacement = "|──(\(sup) / \(sub))──>"
                } else if !sup.isEmpty {
                    replacement = "|──(\(sup))──>"
                } else if !sub.isEmpty {
                    replacement = "|──(\(sub))──>"
                } else {
                    replacement = "⟼"
                }
            default:
                replacement = "⟶"
            }
            
            result = (result as NSString).replacingCharacters(in: match.range, with: replacement)
        }
        return result
    }
    
    private static func cleanArrowLabel(_ label: String) -> String {
        var s = unwrapTextAndFontCommands(label.trimmingCharacters(in: .whitespacesAndNewlines))
        s = replaceSymbolsSinglePass(s)
        s = s.replacingOccurrences(of: "{", with: "").replacingOccurrences(of: "}", with: "")
        s = s.replacingOccurrences(of: "[ \\t]+", with: " ", options: .regularExpression)
        return s.trimmingCharacters(in: .whitespacesAndNewlines)
    }
    
    // MARK: - Math Expression Cleaner
    
    /// Cleans a math expression, stripping wrappers and translating LaTeX symbols
    public static func cleanMathExpression(_ input: String) -> String {
        var str = input
        
        // 1. Unwrap \text{...}, \mathrm{...}, \mathbf{...}, \textcolor{...}{...}
        str = unwrapTextAndFontCommands(str)
        
        // 2. Underbrace & Overbrace: \underbrace{a}_{b} -> a〔b〕
        str = replaceUnderOverBraces(str)
        
        // 3. Extensible arrows: \xrightarrow{训练} -> ──(训练)──>
        str = replaceExtensibleArrows(str)
        
        // 4. Math tag, label, and environments (\begin{aligned}...\end{aligned})
        if let mathTagRegex = Self.mathTagRegex {
            str = mathTagRegex.stringByReplacingMatches(in: str, range: NSRange(location: 0, length: (str as NSString).length), withTemplate: " ($1)")
        }
        if let mathLabelRegex = Self.mathLabelRegex {
            str = mathLabelRegex.stringByReplacingMatches(in: str, range: NSRange(location: 0, length: (str as NSString).length), withTemplate: "")
        }
        if let mathEnvRegex = Self.mathEnvRegex {
            str = mathEnvRegex.stringByReplacingMatches(in: str, range: NSRange(location: 0, length: (str as NSString).length), withTemplate: "")
        }
        
        // Clean alignment tabs (&) while preserving escaped \&
        let escapedAmpPlaceholder = "\u{E001}"
        str = str.replacingOccurrences(of: #"\&"#, with: escapedAmpPlaceholder)
        str = str.replacingOccurrences(of: "&=", with: "=")
                 .replacingOccurrences(of: "& =", with: "=")
                 .replacingOccurrences(of: "&", with: " ")
        str = str.replacingOccurrences(of: escapedAmpPlaceholder, with: "&")
        
        // 5. Structural commands: \boxed, \overset, \underset, \binom, \sqrt, \frac, accents
        str = replaceStructuralMathCommands(str)
        
        // 6. Bracket & sizing cleanups
        str = cleanSizingAndDelimiters(str, isMathMode: true)
        
        // 7. Replace LaTeX symbols using single-pass scanner
        str = replaceSymbolsSinglePass(str)
        if let fontFallbackRegex = Self.fontFallbackRegex {
            str = fontFallbackRegex.stringByReplacingMatches(
                in: str,
                range: NSRange(location: 0, length: (str as NSString).length),
                withTemplate: "$1"
            )
        }
        
        // 8. Convert superscripts: ^{...} and ^x
        if str.contains("^") {
            if let supGroupRegex = Self.supGroupRegex {
                let nsStr = str as NSString
                let matches = supGroupRegex.matches(in: str, range: NSRange(location: 0, length: nsStr.length))
                for match in matches.reversed() {
                    let inner = nsStr.substring(with: match.range(at: 1)).trimmingCharacters(in: .whitespaces)
                    let canMapAll = !inner.isEmpty && inner.allSatisfy { superscriptDict[$0] != nil || $0 == " " || $0 == "," }
                    let converted: String
                    if canMapAll {
                        converted = String(inner.map { superscriptDict[$0] ?? $0 })
                    } else {
                        converted = "⁽\(inner)⁾"
                    }
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
                    } else {
                        str = (str as NSString).replacingCharacters(in: match.range, with: "⁽\(inner)⁾")
                    }
                }
            }
        }
        
        // 9. Convert subscripts: _{...} and _x
        if str.contains("_") {
            if let subGroupRegex = Self.subGroupRegex {
                let nsStr = str as NSString
                let matches = subGroupRegex.matches(in: str, range: NSRange(location: 0, length: nsStr.length))
                for match in matches.reversed() {
                    let inner = nsStr.substring(with: match.range(at: 1)).trimmingCharacters(in: .whitespaces)
                    let canMapAll = !inner.isEmpty && inner.allSatisfy { subscriptDict[$0] != nil || $0 == " " || $0 == "," }
                    let converted: String
                    if canMapAll {
                        converted = String(inner.map { subscriptDict[$0] ?? $0 })
                    } else {
                        converted = "₍\(inner)₎"
                    }
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
                    } else {
                        str = (str as NSString).replacingCharacters(in: match.range, with: "₍\(inner)₎")
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
