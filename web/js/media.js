// --- Helpers ---

/** Validates that a URL uses a safe protocol scheme. Blocks javascript:, data:, vbscript: etc. */
function isSafeURL(url) {
  if (!url) return false;
  const trimmed = String(url).replace(/^[\s\u00A0]+/, "");
  if (trimmed.startsWith("//") || trimmed.startsWith("\\\\")) return false;
  if (trimmed.startsWith("/") || trimmed.startsWith("#") || trimmed.startsWith("./")) return true;
  try {
    const parsed = new URL(trimmed);
    const proto = parsed.protocol.toLowerCase();
    return proto === "http:" || proto === "https:";
  } catch (_) {
    return false;
  }
}

function escapeHtml(str) {
  if (!str) return "";
  return String(str)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#039;");
}

function htmlUnescape(str) {
  if (!str) return "";
  return String(str)
    .replace(/&quot;/g, '"')
    .replace(/&#039;/g, "'")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&amp;/g, "&");
}

function formatRelativeTime(dateStr) {
  if (!dateStr) return "";
  const diff = (Date.now() - new Date(dateStr).getTime()) / 1000;
  if (diff < 60) return "刚刚";
  if (diff < 3600) return `${Math.floor(diff / 60)}分钟前`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}小时前`;
  return `${Math.floor(diff / 86400)}天前`;
}

let fileIconTheme = null;
let fileIconThemePromise = null;

function loadFileIconThemeIfNeeded() {
  if (fileIconTheme || fileIconThemePromise) return fileIconThemePromise;
  fileIconThemePromise = fetch("/icons/symbol-icon-theme.json")
    .then(res => res.json())
    .then(data => { fileIconTheme = data; return data; })
    .catch(err => {
      console.warn("Failed to load file icon theme:", err);
      return null;
    });
  return fileIconThemePromise;
}

if (typeof window !== "undefined") {
  if ("requestIdleCallback" in window) {
    window.requestIdleCallback(() => { loadFileIconThemeIfNeeded(); }, { timeout: 3000 });
  } else {
    setTimeout(() => { loadFileIconThemeIfNeeded(); }, 1500);
  }
}

const fileIconFallback = {
  "go.mod": "go-mod", "go.sum": "go-mod", "package.json": "node",
  "package-lock.json": "node", "dockerfile": "docker", "makefile": "shell",
  "info.plist": "xml", "readme.md": "markdown",
  "go": "go", "swift": "swift", "html": "code-orange", "htm": "code-orange",
  "json": "brackets-yellow", "md": "markdown", "plist": "xml", "xml": "xml",
  "py": "python", "js": "js", "ts": "ts", "jsx": "react", "tsx": "react",
  "css": "sass", "scss": "sass", "sh": "shell", "bash": "shell", "zsh": "shell",
  "yaml": "yaml", "yml": "yaml", "sql": "database", "rs": "rust", "c": "c",
  "cpp": "cplus", "java": "java", "kt": "kotlin", "png": "image", "jpg": "image"
};

function resolveFileIcon(nameOrUrl) {
  if (!nameOrUrl) return null;
  let clean = nameOrUrl.trim().replace(/^file:\/\//, "");
  let filename = clean.split("/").pop().toLowerCase();

  // Fast path: common extensions from synchronous fallback table
  if (fileIconFallback[filename]) return fileIconFallback[filename];
  let ext = filename.split(".").pop();
  if (ext && fileIconFallback[ext]) return fileIconFallback[ext];

  // Secondary path: full icon theme
  if (fileIconTheme) {
    if (fileIconTheme.fileNames && fileIconTheme.fileNames[filename]) {
      return fileIconTheme.fileNames[filename];
    }
    if (ext && fileIconTheme.fileExtensions && fileIconTheme.fileExtensions[ext]) {
      return fileIconTheme.fileExtensions[ext];
    }
  } else {
    loadFileIconThemeIfNeeded();
  }
  return null;
}

const latexSymbolLookup = {
  // Blackboard bold (\mathbb & \mathbbm)
  "mathbb{A}": "𝔸", "mathbb{B}": "𝔹", "mathbb{C}": "ℂ", "mathbf{C}": "ℂ",
  "mathbb{D}": "𝔻", "mathbb{E}": "𝔼", "mathbb{F}": "𝔽", "mathbb{G}": "𝔾",
  "mathbb{H}": "ℍ", "mathbb{I}": "𝕀", "mathbb{J}": "𝕁", "mathbb{K}": "𝕂",
  "mathbb{L}": "𝕃", "mathbb{M}": "𝕄", "mathbb{N}": "ℕ", "mathbf{N}": "ℕ",
  "mathbb{O}": "𝕆", "mathbb{P}": "ℙ", "mathbb{Q}": "ℚ", "mathbf{Q}": "ℚ",
  "mathbb{R}": "ℝ", "mathbf{R}": "ℝ", "mathbb{S}": "𝕊", "mathbb{T}": "𝕋",
  "mathbb{U}": "𝕌", "mathbb{V}": "𝕍", "mathbb{W}": "𝕎", "mathbb{X}": "𝕏",
  "mathbb{Y}": "𝕐", "mathbb{Z}": "ℤ", "mathbf{Z}": "ℤ",
  "mathbb{0}": "𝟘", "mathbb{1}": "𝟙", "mathbbm{1}": "𝟙", "mathbb{k}": "𝕜",

  // Calligraphic (\mathcal) - full A..Z
  "mathcal{A}": "𝒜", "mathcal{B}": "ℬ", "mathcal{C}": "𝒞", "mathcal{D}": "𝒟",
  "mathcal{E}": "ℰ", "mathcal{F}": "ℱ", "mathcal{G}": "𝒢", "mathcal{H}": "ℋ",
  "mathcal{I}": "ℐ", "mathcal{J}": "𝒥", "mathcal{K}": "𝒦", "mathcal{L}": "ℒ",
  "mathcal{M}": "ℳ", "mathcal{N}": "𝒩", "mathcal{O}": "𝒪", "mathcal{P}": "𝒫",
  "mathcal{Q}": "𝒬", "mathcal{R}": "ℛ", "mathcal{S}": "𝒮", "mathcal{T}": "𝒯",
  "mathcal{U}": "𝒰", "mathcal{V}": "𝒱", "mathcal{W}": "𝒲", "mathcal{X}": "𝒳",
  "mathcal{Y}": "𝒴", "mathcal{Z}": "𝒵",

  // Script (\mathscr) - full A..Z
  "mathscr{A}": "𝒜", "mathscr{B}": "ℬ", "mathscr{C}": "𝒞", "mathscr{D}": "𝒟",
  "mathscr{E}": "ℰ", "mathscr{F}": "ℱ", "mathscr{G}": "𝒢", "mathscr{H}": "ℋ",
  "mathscr{I}": "ℐ", "mathscr{J}": "𝒥", "mathscr{K}": "𝒦", "mathscr{L}": "ℒ",
  "mathscr{M}": "ℳ", "mathscr{N}": "𝒩", "mathscr{O}": "𝒪", "mathscr{P}": "𝒫",
  "mathscr{Q}": "𝒬", "mathscr{R}": "ℛ", "mathscr{S}": "𝒮", "mathscr{T}": "𝒯",
  "mathscr{U}": "𝒰", "mathscr{V}": "𝒱", "mathscr{W}": "𝒲", "mathscr{X}": "𝒳",
  "mathscr{Y}": "𝒴", "mathscr{Z}": "𝒵",

  // Fraktur (\mathfrak) - A..Z, a..z
  "mathfrak{A}": "𝔄", "mathfrak{B}": "𝔅", "mathfrak{C}": "ℭ", "mathfrak{D}": "𝔇",
  "mathfrak{E}": "𝔈", "mathfrak{F}": "𝔉", "mathfrak{G}": "𝔊", "mathfrak{H}": "ℌ",
  "mathfrak{I}": "ℑ", "mathfrak{J}": "𝔍", "mathfrak{K}": "𝔎", "mathfrak{L}": "𝔏",
  "mathfrak{M}": "𝔐", "mathfrak{N}": "𝔑", "mathfrak{O}": "𝔒", "mathfrak{P}": "𝔓",
  "mathfrak{Q}": "𝔔", "mathfrak{R}": "ℜ", "mathfrak{S}": "𝔖", "mathfrak{T}": "𝔗",
  "mathfrak{U}": "𝔘", "mathfrak{V}": "𝔙", "mathfrak{W}": "𝔚", "mathfrak{X}": "𝔛",
  "mathfrak{Y}": "𝔜", "mathfrak{Z}": "ℨ",
  "mathfrak{a}": "𝔞", "mathfrak{b}": "𝔟", "mathfrak{c}": "𝔠", "mathfrak{d}": "𝔡",
  "mathfrak{e}": "𝔢", "mathfrak{f}": "𝔣", "mathfrak{g}": "𝔤", "mathfrak{h}": "𝔥",
  "mathfrak{i}": "𝔦", "mathfrak{j}": "𝔧", "mathfrak{k}": "𝔨", "mathfrak{l}": "𝔩",
  "mathfrak{m}": "𝔪", "mathfrak{n}": "𝔫", "mathfrak{o}": "𝔬", "mathfrak{p}": "𝔭",
  "mathfrak{q}": "𝔮", "mathfrak{r}": "𝔯", "mathfrak{s}": "𝔰", "mathfrak{t}": "𝔱",
  "mathfrak{u}": "𝔲", "mathfrak{v}": "𝔳", "mathfrak{w}": "𝔴", "mathfrak{x}": "𝔵",
  "mathfrak{y}": "𝔶", "mathfrak{z}": "𝔷",

  // Delimiters, Brackets & Norms
  "langle": "⟨", "rangle": "⟩",
  "lfloor": "⌊", "rfloor": "⌋",
  "lceil": "⌈", "rceil": "⌉",
  "ulcorner": "⌜", "urcorner": "⌝", "llcorner": "⌞", "lrcorner": "⌟",
  "lVert": "‖", "rVert": "‖", "Vert": "‖",
  "lvert": "|", "rvert": "|", "vert": "|",
  "mid": "∣", "nmid": "∤", "backslash": "\\",

  // Long & squiggly arrows
  "longleftrightarrow": "⟷", "Longleftrightarrow": "⟺",
  "leftrightsquigarrow": "↭", "rightsquigarrow": "⇝",
  "twoheadrightarrow": "↠", "twoheadleftarrow": "↞",
  "longrightarrow": "⟶", "longleftarrow": "⟵",
  "Longrightarrow": "⟹", "Longleftarrow": "⟸",
  "rightarrowtail": "↣", "leftarrowtail": "↢",
  "circlearrowleft": "↺", "circlearrowright": "↻",
  "curvearrowleft": "↶", "curvearrowright": "↷",
  "longmapsto": "⟼", "mapsfrom": "↤", "leadsto": "↝",

  // Standard arrows & harpoons
  "rightleftharpoons": "⇌", "rightharpoonup": "⇀", "rightharpoondown": "⇁",
  "leftharpoonup": "↼", "leftharpoondown": "↽",
  "hookrightarrow": "↪", "hookleftarrow": "↩",
  "rightarrow": "→", "leftarrow": "←",
  "leftrightarrow": "↔", "Rightarrow": "⇒", "Leftarrow": "⇐", "Leftrightarrow": "⇔",
  "to": "→", "gets": "←", "implies": "⇒", "impliedby": "⇐", "iff": "⇔",
  "uparrow": "↑", "downarrow": "↓", "updownarrow": "↕",
  "Uparrow": "⇑", "Downarrow": "⇓", "Updownarrow": "⇕",
  "nearrow": "↗", "searrow": "↘", "swarrow": "↙", "nwarrow": "↖",
  "mapsto": "↦",

  // Comparisons & Relations
  "fallingdotseq": "≒", "risingdotseq": "≓",
  "lessapprox": "⪅", "gtrapprox": "⪆",
  "sqsubseteq": "⊑", "sqsupseteq": "⊒", "sqsubset": "⊏", "sqsupset": "⊐",
  "triangleq": "≜", "coloneqq": "≔", "eqqcolon": "≕", "coloneq": "≔", "eqcolon": "≕", "defeq": "≔",
  "leqslant": "≤", "geqslant": "≥", "leq": "≤", "geq": "≥",
  "le": "≤", "ge": "≥", "neq": "≠", "ne": "≠",
  "approx": "≈", "simeq": "≃", "ncong": "≇", "cong": "≅", "equiv": "≡", "nequiv": "≢",
  "propto": "∝", "asymp": "≍", "doteq": "≐", "circeq": "≗",
  "bowtie": "⋈", "ltimes": "⋉", "rtimes": "⋊",
  "preceq": "⪯", "succeq": "⪰", "prec": "≺", "succ": "≻",
  "lesssim": "≲", "gtrsim": "≳", "napprox": "≉", "nsim": "≁", "sim": "∼",
  "ll": "≪", "gg": "≫", "parallel": "∥", "nparallel": "∦", "perp": "⊥",
  "subset": "⊂", "supset": "⊃", "nsubset": "⊄",
  "subseteq": "⊆", "supseteq": "⊇", "nsubseteq": "⊈", "nsupseteq": "⊉",
  "subsetneq": "⊊", "supsetneq": "⊋",

  // Set & Logic
  "smallsetminus": "∖", "complement": "∁", "varnothing": "∅",
  "emptyset": "∅", "empty": "∅", "setminus": "∖",
  "therefore": "∴", "because": "∵",
  "notin": "∉", "in": "∈", "ni": "∋", "owns": "∋",
  "forall": "∀", "exists": "∃", "nexists": "∄",
  "lnot": "¬", "neg": "¬", "land": "∧", "wedge": "∧", "lor": "∨", "vee": "∨",
  "top": "⊤", "bot": "⊥", "vdash": "⊢", "dashv": "⊣", "vDash": "⊨", "Vdash": "⊩", "models": "⊨",
  "bigcup": "⋃", "bigcap": "⋂", "bigsqcup": "⨆", "biguplus": "⨄", "bigvee": "⋁", "bigwedge": "⋀",
  "sqcup": "⊔", "sqcap": "⊓", "uplus": "⊎", "cup": "∪", "cap": "∩",

  // Operators & Calculus
  "bigoplus": "⨁", "bigotimes": "⨂", "bigodot": "⨀",
  "boxtimes": "⊠", "boxplus": "⊞", "boxminus": "⊟", "boxdot": "⊡",
  "times": "×", "div": "÷", "pm": "±", "mp": "∓",
  "cdot": "·", "cdots": "⋯", "ldots": "…", "dotsc": "…", "dotsb": "⋯", "dotsm": "⋯", "dotsi": "⋯", "dots": "…",
  "vdots": "⋮", "ddots": "⋱",
  "bullet": "•", "circ": "∘", "star": "⋆", "ast": "∗",
  "oplus": "⊕", "ominus": "⊖", "otimes": "⊗", "oslash": "⊘", "odot": "⊙",
  "dagger": "†", "ddagger": "‡", "wr": "≀", "colon": ":", "prime": "′", "checkmark": "✓",
  "oiiint": "∰", "oiint": "∯", "iiint": "∭", "iint": "∬", "oint": "∮", "int": "∫",
  "sum": "∑", "prod": "∏", "coprod": "∐",
  "partial": "∂", "nabla": "∇", "infty": "∞", "surd": "√", "sqrt": "√",
  "angle": "∠", "measuredangle": "∡", "sphericalangle": "∢",
  "blacktriangledown": "▼", "bigtriangledown": "▽", "blacktriangle": "▲", "bigtriangleup": "△",
  "triangleleft": "◁", "triangleright": "▷", "triangle": "△",
  "blacksquare": "■", "diamond": "◇", "Diamond": "◇", "lozenge": "◊", "square": "□", "Box": "□",
  "degree": "°", "hbar": "ℏ", "ell": "ℓ", "aleph": "ℵ", "beth": "ℶ", "gimel": "ℷ", "daleth": "ℸ",
  "wp": "℘", "Re": "ℜ", "Im": "ℑ", "mho": "℧",

  // Standard Math Functions
  "limsup": "lim sup", "liminf": "lim inf", "argmax": "arg max", "argmin": "arg min",
  "arcsin": "arcsin", "arccos": "arccos", "arctan": "arctan",
  "sinh": "sinh", "cosh": "cosh", "tanh": "tanh", "coth": "coth",
  "lim": "lim", "sup": "sup", "inf": "inf", "max": "max", "min": "min",
  "arg": "arg", "det": "det", "dim": "dim", "ker": "ker", "hom": "hom",
  "deg": "deg", "gcd": "gcd", "lcm": "lcm",
  "exp": "exp", "log": "log", "ln": "ln", "lg": "lg",
  "sin": "sin", "cos": "cos", "tan": "tan", "cot": "cot", "sec": "sec", "csc": "csc",
  "Pr": "Pr", "bmod": "mod",

  // Layout / Spacing / Style No-ops
  "displaystyle": "", "scriptscriptstyle": "", "scriptstyle": "", "textstyle": "",
  "nolimits": "", "limits": "", "noindent": "",
  "qquad": "  ", "quad": " ", "enspace": " ", "thinspace": " ",

  // Greek capital letters
  "Gamma": "Γ", "Delta": "Δ", "Theta": "Θ", "Lambda": "Λ", "Xi": "Ξ",
  "Pi": "Π", "Sigma": "Σ", "Upsilon": "Υ", "Phi": "Φ", "Psi": "Ψ", "Omega": "Ω",

  // Greek lowercase letters
  "alpha": "α", "beta": "β", "gamma": "γ", "delta": "δ",
  "varepsilon": "ε", "epsilon": "ϵ", "zeta": "ζ", "eta": "η",
  "vartheta": "ϑ", "theta": "θ", "iota": "ι", "kappa": "κ",
  "lambda": "λ", "mu": "μ", "nu": "ν", "xi": "ξ", "omicron": "ο", "pi": "π",
  "varpi": "ϖ", "varrho": "ϱ", "rho": "ρ", "varsigma": "ς", "sigma": "σ",
  "tau": "τ", "upsilon": "υ", "varphi": "φ", "phi": "ϕ",
  "chi": "χ", "psi": "ψ", "omega": "ω"
};

const wordBoundaryCommands = new Set([
  "to", "in", "ni", "le", "ge", "ne", "empty", "gets", "sim",
  "mid", "ll", "gg", "cup", "cap", "neg", "top", "bot", "vee",
  "div", "pm", "mp", "ast", "wr", "int", "sum", "prod", "ell",
  "wp", "Re", "Im", "Box", "lim", "sup", "inf", "max", "min",
  "arg", "det", "dim", "ker", "hom", "deg", "gcd", "lcm", "exp",
  "log", "ln", "lg", "sin", "cos", "tan", "cot", "sec", "csc", "Pr"
]);

function replaceSymbolsSinglePass(str) {
  if (!str || !str.includes("\\")) return str;
  let out = "";
  let i = 0;
  const len = str.length;

  while (i < len) {
    if (str[i] === "\\") {
      let j = i + 1;
      while (j < len && ((str.charCodeAt(j) >= 65 && str.charCodeAt(j) <= 90) || (str.charCodeAt(j) >= 97 && str.charCodeAt(j) <= 122))) {
        j++;
      }
      const cmd = str.slice(i + 1, j);
      let fullKey = cmd;
      let nextJ = j;

      if (j < len && str[j] === "{") {
        const closeIdx = str.indexOf("}", j + 1);
        if (closeIdx !== -1 && closeIdx - j <= 12) {
          const paramCandidate = cmd + "{" + str.slice(j + 1, closeIdx) + "}";
          if (Object.prototype.hasOwnProperty.call(latexSymbolLookup, paramCandidate)) {
            fullKey = paramCandidate;
            nextJ = closeIdx + 1;
          }
        }
      }

      if (fullKey && Object.prototype.hasOwnProperty.call(latexSymbolLookup, fullKey)) {
        if (wordBoundaryCommands.has(fullKey) && nextJ < len &&
            ((str.charCodeAt(nextJ) >= 65 && str.charCodeAt(nextJ) <= 90) || (str.charCodeAt(nextJ) >= 97 && str.charCodeAt(nextJ) <= 122))) {
          out += str[i];
          i++;
        } else {
          out += latexSymbolLookup[fullKey];
          i = nextJ;
        }
      } else {
        out += str[i];
        i++;
      }
    } else {
      out += str[i];
      i++;
    }
  }
  return out;
}

const supMap = {
  "0": "⁰", "1": "¹", "2": "²", "3": "³", "4": "⁴", "5": "⁵", "6": "⁶", "7": "⁷", "8": "⁸", "9": "⁹",
  "+": "⁺", "-": "⁻", "=": "⁼", "(": "⁽", ")": "⁾",
  "n": "ⁿ", "i": "ⁱ", "j": "ʲ", "a": "ᵃ", "b": "ᵇ", "c": "ᶜ", "d": "ᵈ", "e": "ᵉ", "f": "ᶠ", "g": "ᵍ",
  "h": "ʰ", "k": "ᵏ", "l": "ˡ", "m": "ᵐ", "o": "ᵒ", "p": "ᵖ", "r": "ʳ", "s": "ˢ", "t": "ᵗ", "u": "ᵘ",
  "v": "ᵛ", "w": "ʷ", "x": "ˣ", "y": "ʸ", "z": "ᶻ",
  "A": "ᴬ", "B": "ᴮ", "D": "ᴰ", "E": "ᴱ", "G": "ᴳ", "H": "ᴴ", "I": "ᴵ", "J": "ᴶ", "K": "ᴷ", "L": "ᴸ",
  "M": "ᴹ", "N": "ᴺ", "O": "ᴼ", "P": "ᴾ", "R": "ᴿ", "T": "ᵀ", "U": "ᵁ", "V": "ⱽ", "W": "ᵂ",
  "⊤": "ᵀ", "∘": "°", "′": "′", "*": "*", "∗": "*"
};
const subMap = {
  "0": "₀", "1": "₁", "2": "₂", "3": "₃", "4": "₄", "5": "₅", "6": "₆", "7": "₇", "8": "₈", "9": "₉",
  "+": "₊", "-": "₋", "=": "₌", "(": "₍", ")": "₎",
  "a": "ₐ", "e": "ₑ", "h": "ₕ", "i": "ᵢ", "j": "ⱼ", "k": "ₖ", "l": "ₗ", "m": "ₘ", "n": "ₙ", "o": "ₒ",
  "p": "ₚ", "r": "ᵣ", "s": "ₛ", "t": "ₜ", "u": "ᵤ", "v": "ᵥ", "x": "ₓ",
  "β": "ᵦ", "γ": "ᵧ", "ρ": "ᵨ", "φ": "ᵩ", "χ": "ᵪ"
};

function unwrapTextAndFontCommands(str) {
  if (!str || !str.includes("\\")) return str;
  str = str.replace(/([_^])\\(?:text|textbf|textit|textrm|textsf|texttt|textsc|mathrm|mathbf|mathit|mathsf|mathtt|mathbbm|operatorname\*?|pmb|boldsymbol|bm|bold|emph|mbox|hbox)\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}/g, "$1{$2}");
  str = str.replace(/\\(?:textcolor|color)\{[^{}]*\}\{([^{}]*)\}|\\color\{[^{}]*\}/g, "$1");
  for (let pass = 0; pass < 4; pass++) {
    const next = str.replace(/\\(?:text|textbf|textit|textrm|textsf|texttt|textsc|mathrm|mathbf|mathit|mathsf|mathtt|mathbbm|operatorname\*?|pmb|boldsymbol|bm|bold|emph|mbox|hbox)\{([^{}]*)\}/g, "$1");
    if (next === str) break;
    str = next;
  }
  return str;
}

function replaceUnderOverBraces(str) {
  if (!str || !str.includes("brace")) return str;
  const underRe = /\\underbrace\s*\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}\s*(?:_\s*\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}|_\s*([a-zA-Z0-9])|\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\})?/g;
  const overRe = /\\overbrace\s*\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}\s*(?:\^\s*\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}|\^\s*([a-zA-Z0-9])|\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\})?/g;
  for (let pass = 0; pass < 4; pass++) {
    const next = str
      .replace(underRe, (_, base, g2, g3, g4) => {
        const b = (base || "").trim().replace(/[ \t]+/g, " ");
        const note = (g2 || g3 || g4 || "").trim().replace(/[ \t]+/g, " ");
        return note ? `${b}〔${note}〕` : b;
      })
      .replace(overRe, (_, base, g2, g3, g4) => {
        const b = (base || "").trim().replace(/[ \t]+/g, " ");
        const note = (g2 || g3 || g4 || "").trim().replace(/[ \t]+/g, " ");
        return note ? `${b}〔${note}〕` : b;
      });
    if (next === str) break;
    str = next;
  }
  return str;
}

function cleanArrowLabel(label) {
  let s = unwrapTextAndFontCommands((label || "").trim());
  s = replaceSymbolsSinglePass(s);
  return s.replace(/[{}]/g, "").replace(/[ \t]+/g, " ").trim();
}

function replaceExtensibleArrows(str) {
  if (!str || !str.includes("\\x")) return str;
  return str.replace(/\\x(rightarrow|leftarrow|Rightarrow|Leftarrow|leftrightarrow|Leftrightarrow|mapsto)(?:\[([^\]]*)\])?\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}/g, (_, arrowType, rawSub, rawSup) => {
    const sup = cleanArrowLabel(rawSup);
    const sub = rawSub ? cleanArrowLabel(rawSub) : "";
    const label = (sup && sub) ? `${sup} / ${sub}` : (sup || sub);
    switch (arrowType) {
      case "rightarrow": return label ? `──(${label})──>` : "⟶";
      case "leftarrow": return label ? `<──(${label})──` : "⟵";
      case "Rightarrow": return label ? `══(${label})══>` : "⟹";
      case "Leftarrow": return label ? `<══(${label})══` : "⟸";
      case "leftrightarrow": return label ? `<──(${label})──>` : "⟷";
      case "Leftrightarrow": return label ? `<══(${label})══>` : "⟺";
      case "mapsto": return label ? `|──(${label})──>` : "⟼";
      default: return "⟶";
    }
  });
}

function replaceStructuralMathCommands(str) {
  if (!str || !str.includes("\\")) return str;
  if (str.includes("\\not")) {
    str = str.replace(/\\not\s*(\\(?:in|equiv|sim|approx|cong|subset|subseteq|supset|supseteq|parallel|mid)\b|=)/g, (_, op) => {
      switch (op) {
        case "\\in": return "∉";
        case "=": return "≠";
        case "\\equiv": return "≢";
        case "\\sim": return "≁";
        case "\\approx": return "≉";
        case "\\cong": return "≇";
        case "\\subset": return "⊄";
        case "\\subseteq": return "⊈";
        case "\\supset": return "⊅";
        case "\\supseteq": return "⊉";
        case "\\parallel": return "∦";
        case "\\mid": return "∤";
        default: return op;
      }
    });
  }
  str = str.replace(/\\boxed\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}/g, "[$1]");
  str = str.replace(/\\(?:cancel|bcancel|xcancel|sout)\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}/g, "~~$1~~");
  str = str.replace(/\\pmod\{([^{}]*)\}/g, "(mod $1)");
  str = str.replace(/\\overset\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}/g, "$2 ($1)");
  str = str.replace(/\\underset\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}/g, "$2 ($1)");
  str = str.replace(/\\(?:d|t)?binom\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}/g, "C($1, $2)");
  str = str.replace(/\\sqrt\[([^\]]*)\]\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}/g, (_, nRaw, rad) => {
    const nSup = String(nRaw).split("").map(c => supMap[c] || c).join("");
    return `${nSup}√(${rad})`;
  });
  for (let pass = 0; pass < 2; pass++) {
    const next = str.replace(/\\sqrt\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}/g, "√($1)");
    if (next === str) break;
    str = next;
  }
  for (let pass = 0; pass < 3; pass++) {
    const next = str.replace(/\\(?:d|t|c)?frac\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}/g, "$1 / $2");
    if (next === str) break;
    str = next;
  }
  str = str.replace(/\\(widehat|hat|widetilde|tilde|overline|bar|overrightarrow|vec|ddot|dot|underline)\{([^{}]*)\}/g, (_, cmd, rawBase) => {
    const base = (rawBase || "").trim();
    if (!base) return "";
    const map = {
      hat: "\u0302", widehat: "\u0302",
      tilde: "\u0303", widetilde: "\u0303",
      bar: "\u0304", overline: "\u0304",
      vec: "\u20D7", overrightarrow: "\u20D7",
      dot: "\u0307", ddot: "\u0308", underline: "\u0332"
    };
    const comb = map[cmd] || "";
    if (Array.from(base).length === 1 || cmd === "overline" || cmd === "underline") {
      return Array.from(base).map(c => c + comb).join("");
    }
    return base + comb;
  });
  return str;
}

function cleanSizingAndDelimiters(str, isMathMode) {
  if (!str) return "";
  str = str
    .replace(/\\left\./g, "")
    .replace(/\\right\./g, "")
    .replace(/\\left\\\|/g, "‖")
    .replace(/\\right\\\|/g, "‖")
    .replace(/\\left\|/g, "|")
    .replace(/\\right\|/g, "|")
    .replace(/\\left\(/g, "(")
    .replace(/\\right\)/g, ")")
    .replace(/\\left\[/g, "[")
    .replace(/\\right\]/g, "]")
    .replace(/\\left\\\{/g, "{")
    .replace(/\\right\\\}/g, "}");
  str = str.replace(/\\(?:left|right|middle|bigl|bigr|Bigl|Bigr|biggl|biggr|Biggl|Biggr|big|Big|bigg|Bigg)\s*(?=[(\[{|⟨⟩⌊⌋⌈⌉‖/\\])/g, "");
  if (isMathMode) {
    str = str
      .replace(/\\\|/g, "‖")
      .replace(/\\\{/g, "{")
      .replace(/\\\}/g, "}")
      .replace(/\\%/g, "%")
      .replace(/\\_/g, "_")
      .replace(/\\,/g, " ")
      .replace(/\\;/g, " ")
      .replace(/\\:/g, " ")
      .replace(/\\!/g, "")
      .replace(/\\quad/g, " ")
      .replace(/\\qquad/g, "  ");
  }
  return str;
}

function convertSupGroup(inner) {
  const trimmed = (inner || "").trim();
  if (!trimmed) return "";
  const chars = Array.from(trimmed);
  const canMapAll = chars.every(c => supMap[c] !== undefined || c === " " || c === ",");
  return canMapAll ? chars.map(c => supMap[c] || c).join("") : `⁽${trimmed}⁾`;
}

function convertSubGroup(inner) {
  const trimmed = (inner || "").trim();
  if (!trimmed) return "";
  const chars = Array.from(trimmed);
  const canMapAll = chars.every(c => subMap[c] !== undefined || c === " " || c === ",");
  return canMapAll ? chars.map(c => subMap[c] || c).join("") : `₍${trimmed}₎`;
}

function replaceBareOperatorLimits(str) {
  if (!str || (!str.includes("_") && !str.includes("^"))) return str;
  return str.replace(/(\b(?:lim sup|lim inf|arg max|arg min|lim|max|min|sup|inf)|[∑∏∐∫∬∭∮∯∰⋃⋂⨁⨂⨀⨆⨄⋁⋀])\s*(?:_\{([^{}]+)\}|_([0-9a-zA-Z]))?(?:\s*(?:\^\{([^{}]+)\}|\^([0-9a-zA-Z+\-*⊤∘′∗])))?/g, (match, op, subG, subS, supG, supS) => {
    if (!subG && !subS && !supG && !supS) return match;
    const subPart = subG ? convertSubGroup(subG) : (subS ? (subMap[subS] || `₍${subS}₎`) : "");
    const supPart = supG ? convertSupGroup(supG) : (supS ? (supMap[supS] || `⁽${supS}⁾`) : "");
    return `${op}${subPart}${supPart}`;
  });
}

function cleanMathExpr(str) {
  if (!str) return "";
  str = str.replace(/\\\\/g, "\n");
  str = unwrapTextAndFontCommands(str);
  str = replaceUnderOverBraces(str);
  str = replaceExtensibleArrows(str);
  str = str.replace(/\\tag\*?\{([^{}]*)\}/g, " ($1)");
  str = str.replace(/\\label\{([^{}]*)\}/g, "");
  str = str.replace(/\\(?:begin|end)\{(?:aligned|align\*?|matrix|bmatrix|pmatrix|vmatrix|Vmatrix|Bmatrix|cases|split|gather\*?|multline\*?|array|equation\*?|subequations)\}(?:\{[^{}]*\})?/g, "");
  str = str.replace(/\\&/g, "\uE001").replace(/&\s*=/g, "=").replace(/&/g, " ").replace(/\uE001/g, "&");
  str = replaceStructuralMathCommands(str);
  str = cleanSizingAndDelimiters(str, true);
  str = replaceSymbolsSinglePass(str);
  str = str.replace(/\\(?:mathcal|mathscr|mathbb|mathfrak|mathbbm)\{([^{}]*)\}/g, "$1");
  str = str.replace(/\^\{([^{}]+)\}/g, (_, chars) => convertSupGroup(chars));
  str = str.replace(/\^([0-9a-zA-Z+\-*⊤∘′∗])/g, (_, c) => supMap[c] || `⁽${c}⁾`);
  str = str.replace(/_\{([^{}]+)\}/g, (_, chars) => convertSubGroup(chars));
  str = str.replace(/_([0-9a-zA-Zβγρφχ])/g, (_, c) => subMap[c] || `₍${c}₎`);
  return str.trim();
}

function processMathSymbols(text) {
  if (!text) return "";
  if (!text.includes("\\") && !text.includes("$")) return text;

  const codeBlocks = [];
  text = text.replace(/```[a-zA-Z0-9_-]*\n[\s\S]*?```/g, m => {
    codeBlocks.push(m);
    return `XXAGYBLOCKTOKEN${codeBlocks.length - 1}XX`;
  });
  const inlineCodes = [];
  text = text.replace(/`[^`\n]+`/g, m => {
    inlineCodes.push(m);
    return `XXAGYINLINETOKEN${inlineCodes.length - 1}XX`;
  });

  text = text.replace(/\$\$([\s\S]*?)\$\$/g, (_, m) => cleanMathExpr(m));
  text = text.replace(/\\\[([\s\S]*?)\\\]/g, (_, m) => cleanMathExpr(m));
  text = text.replace(/(?<!\\)\$(?!\s)([^$\n]+?)(?<!\s)(?<!\\)\$/g, (_, m) => cleanMathExpr(m));
  text = text.replace(/\\\(([\s\S]*?)\\\)/g, (_, m) => cleanMathExpr(m));

  if (text.includes("\\")) {
    text = unwrapTextAndFontCommands(text);
    text = replaceUnderOverBraces(text);
    text = replaceExtensibleArrows(text);
    text = replaceStructuralMathCommands(text);
    text = cleanSizingAndDelimiters(text, false);
    text = replaceSymbolsSinglePass(text);
    text = text.replace(/\\(?:mathcal|mathscr|mathbb|mathfrak|mathbbm)\{([^{}]*)\}/g, "$1");
    text = replaceBareOperatorLimits(text);
  }

  inlineCodes.forEach((c, idx) => {
    text = text.replace(`XXAGYINLINETOKEN${idx}XX`, c);
  });
  codeBlocks.forEach((c, idx) => {
    text = text.replace(`XXAGYBLOCKTOKEN${idx}XX`, c);
  });
  return text;
}

// --- Media & Image Handling ---

/**
 * Normalizes raw image paths/URIs into a URL loadable by the browser.
 * Converts local filesystem paths (/Users/..., file:///..., etc.) into /api/v1/files/raw with auth token.
 */
function resolveMediaRawUrl(rawPath) {
  if (!rawPath) return "";
  let clean = String(rawPath).trim();
  if (clean.startsWith("MEDIA:")) {
    clean = clean.slice(6).trim();
  }
  // Strip enclosing quotes, backticks, or brackets
  clean = clean.replace(/^[`"'<(\[]+|[`>"')\]]+$/g, "");
  
  if (clean.startsWith("data:image/") || clean.startsWith("blob:")) {
    return clean;
  }
  if (clean.startsWith("http://") || clean.startsWith("https://")) {
    return clean;
  }
  if (clean.startsWith("file://")) {
    clean = clean.slice(7);
  }
  
  // C-1: Same-origin requests automatically transmit the HttpOnly session cookie (agy_dt).
  // Long-lived tokens are never appended to URL query parameters.
  const params = new URLSearchParams();
  params.set("uri", clean);
  return `/api/v1/files/raw?${params.toString()}`;
}

/**
 * Returns candidate thumbnail URL. If original path does not have _thumb,
 * replaces .ext with _thumb.ext.
 */
function resolveThumbnailRawUrl(originalPath) {
  if (!originalPath) return "";
  let clean = String(originalPath).trim();
  if (clean.startsWith("MEDIA:")) clean = clean.slice(6).trim();
  clean = clean.replace(/^[`"'<(\[]+|[`>"')\]]+$/g, "");

  if (clean.startsWith("data:image/") || clean.startsWith("blob:")) {
    return clean;
  }
  if (clean.startsWith("http://") || clean.startsWith("https://")) {
    return resolveMediaRawUrl(clean);
  }

  // If path already contains _thumb, use as is
  if (/_thumb\.[a-zA-Z0-9]+$/i.test(clean)) {
    return resolveMediaRawUrl(clean);
  }

  // Try companion _thumb file
  const thumbPath = clean.replace(/\.([a-zA-Z0-9]+)$/, "_thumb.$1");
  return resolveMediaRawUrl(thumbPath);
}

/**
 * Extracts a readable filename from an image path/URI.
 */
function extractImageFileName(rawPath) {
  if (!rawPath) return "图片";
  let clean = String(rawPath).trim();
  if (clean.startsWith("MEDIA:")) clean = clean.slice(6).trim();
  clean = clean.replace(/^[`"'<(\[]+|[`>"')\]]+$/g, "");
  if (clean.startsWith("file://")) clean = clean.slice(7);
  if (clean.includes("?")) clean = clean.split("?")[0];
  const parts = clean.split("/");
  const last = parts.pop() || "";
  return last || "图片";
}

/**
 * Checks if a URL or filename points to an image.
 */
function isImageResource(url) {
  if (!url) return false;
  const clean = url.split("?")[0].toLowerCase();
  return clean.endsWith(".png") || clean.endsWith(".jpg") || clean.endsWith(".jpeg") ||
         clean.endsWith(".webp") || clean.endsWith(".gif") || clean.endsWith(".svg") ||
         clean.endsWith(".bmp") || clean.endsWith(".ico") || clean.startsWith("data:image/");
}
window.isImageResource = isImageResource;

function isPreviewableDocumentResource(url) {
  if (!url) return false;
  const clean = url.split("?")[0].split("#")[0].toLowerCase();
  const docExts = [
    ".pdf", ".pptx", ".ppt", ".key", ".docx", ".doc", ".xlsx", ".xls",
    ".numbers", ".pages", ".mp4", ".mov", ".m4v", ".webm", ".avi", ".zip", ".tar", ".gz"
  ];
  return docExts.some(ext => clean.endsWith(ext));
}

/**
 * Generates the HTML for an image thumbnail card.
 */
function buildImageThumbnailCard(originalPath, thumbPath, altText) {
  if (!originalPath) return "";
  const origRawUrl = resolveMediaRawUrl(originalPath);
  const thumbRawUrl = thumbPath ? resolveMediaRawUrl(thumbPath) : resolveThumbnailRawUrl(originalPath);
  const fileName = extractImageFileName(originalPath);
  const alt = altText ? altText.trim() : fileName;

  return `
    <div class="image-thumb-card" 
         data-action="open-lightbox" 
         data-original-url="${escapeHtml(origRawUrl)}" 
         data-thumb-url="${escapeHtml(thumbRawUrl)}" 
         data-title="${escapeHtml(fileName)}" 
         data-alt="${escapeHtml(alt)}"
         tabindex="0"
         role="button"
         aria-label="查看图片 ${escapeHtml(fileName)}">
      <div class="image-thumb-media">
        <img src="${escapeHtml(thumbRawUrl)}" 
             data-original-src="${escapeHtml(origRawUrl)}" 
             alt="${escapeHtml(alt)}" 
             class="image-thumb-img" 
             loading="lazy" 
             onload="handleThumbnailLoad(this)" 
             onerror="handleThumbnailError(this)" />
        <div class="image-thumb-spinner">
          <div class="ios-spinner small"></div>
        </div>
        <div class="image-thumb-error">
          <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <rect x="3" y="3" width="18" height="18" rx="2" ry="2"></rect>
            <circle cx="8.5" cy="8.5" r="1.5"></circle>
            <polyline points="21 15 16 10 5 21"></polyline>
          </svg>
          <span>无法加载图片</span>
        </div>
      </div>
      <div class="image-thumb-bar">
        <div class="image-thumb-meta">
          <span class="image-thumb-name" title="${escapeHtml(fileName)}">${escapeHtml(fileName)}</span>
          <span class="image-thumb-dimensions"></span>
          <span class="image-thumb-badge hidden">长图</span>
        </div>
        <div class="image-thumb-zoom-pill">
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="11" cy="11" r="8"></circle>
            <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
            <line x1="11" y1="8" x2="11" y2="14"></line>
            <line x1="8" y1="11" x2="14" y2="11"></line>
          </svg>
          <span>放大查看</span>
        </div>
      </div>
    </div>
  `;
}

function buildAgentEmbedCard(src) {
  if (!src) return "";
  let clean = String(src).trim().replace(/^[`"'<(\[]+|[`>"')\]]+$/g, "");
  const fileName = extractImageFileName(clean);
  const cardId = "agent_embed_" + Math.random().toString(36).substring(2, 9);
  const title = (fileName && fileName !== "interactive_preview.html" && fileName !== "widget.html") ? fileName : "交互预览";

  setTimeout(() => {
    if (window.loadAgentEmbedContent) {
      window.loadAgentEmbedContent(cardId, clean);
    }
  }, 10);

  return `
    <div class="agent-embed-card" id="${cardId}" data-src="${escapeHtml(clean)}">
      <div class="agent-embed-header">
        <div class="agent-embed-title-wrap">
          <svg class="agent-embed-icon" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
            <line x1="4" y1="21" x2="4" y2="14"></line>
            <line x1="4" y1="10" x2="4" y2="3"></line>
            <line x1="12" y1="21" x2="12" y2="12"></line>
            <line x1="12" y1="8" x2="12" y2="3"></line>
            <line x1="20" y1="21" x2="20" y2="16"></line>
            <line x1="20" y1="12" x2="20" y2="3"></line>
            <line x1="1" y1="14" x2="7" y2="14"></line>
            <line x1="9" y1="8" x2="15" y2="8"></line>
            <line x1="17" y1="16" x2="23" y2="16"></line>
          </svg>
          <span class="agent-embed-title">${escapeHtml(title)}</span>
          <span class="agent-embed-badge">INTERACTIVE</span>
        </div>
        <div class="agent-embed-actions">
          <button class="agent-embed-action-btn" onclick="refreshAgentEmbedCard('${escapeHtml(cardId)}')" title="刷新交互组件" type="button">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M23 4v6h-6"></path>
              <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"></path>
            </svg>
          </button>
          <button class="agent-embed-action-btn" onclick="toggleFullscreenAgentEmbed('${escapeHtml(cardId)}')" title="全屏查看" type="button">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M8 3H5a2 2 0 0 0-2 2v3m18 0V5a2 2 0 0 0-2-2h-3m0 18h3a2 2 0 0 0 2-2v-3M3 16v3a2 2 0 0 0 2 2h3"></path>
            </svg>
          </button>
        </div>
      </div>
      <div class="agent-embed-viewport">
        <iframe class="agent-embed-frame" sandbox="allow-scripts" src="about:blank"></iframe>
      </div>
    </div>
  `;
}

window.loadAgentEmbedContent = function(cardId, src) {
  const card = document.getElementById(cardId);
  if (!card) return;
  const frame = card.querySelector(".agent-embed-frame");
  if (!frame) return;

  const params = new URLSearchParams();
  params.set("uri", src);
  fetch(`/api/v1/files/content?${params.toString()}`)
    .then(res => {
      if (!res.ok) throw new Error("HTTP " + res.status);
      return res.json();
    })
    .then(data => {
      let rawHtml = data.content || "";
      rawHtml = rawHtml.replace(/<meta\s+[^>]*?name=["']viewport["'][^>]*>/gi, "");
      const isDark = document.documentElement.classList.contains("dark") || window.matchMedia("(prefers-color-scheme: dark)").matches;
      const themeCss = `
        <style>
          :root {
            --background: ${isDark ? "#18181b" : "#ffffff"};
            --foreground: ${isDark ? "#f4f4f5" : "#18181b"};
            --muted-foreground: ${isDark ? "#a1a1aa" : "#71717a"};
            --card: ${isDark ? "#27272a" : "#f4f4f5"};
            --sidebar: ${isDark ? "#18181b" : "#fafafa"};
            --border: ${isDark ? "rgba(255, 255, 255, 0.12)" : "rgba(0, 0, 0, 0.10)"};
            --primary: ${isDark ? "#6366f1" : "#4f46e5"};
            --primary-foreground: #ffffff;
            --secondary: ${isDark ? "#27272a" : "#e4e4e7"};
            --secondary-foreground: ${isDark ? "#f4f4f5" : "#18181b"};
            --accent: ${isDark ? "rgba(255, 255, 255, 0.08)" : "rgba(0, 0, 0, 0.05)"};
            color-scheme: ${isDark ? "dark" : "light"};
          }
          * { box-sizing: border-box; }
          html, body {
            margin: 0; padding: 8px; background: transparent !important;
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            -webkit-text-size-adjust: 100%;
            touch-action: pan-x pan-y;
            overscroll-behavior: none;
          }
          img, svg, video { max-width: 100% !important; }
        </style>
      `;
      const meta = `<meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, minimum-scale=1.0, user-scalable=no, viewport-fit=cover">`;
      const gestureScript = `<script>
        document.addEventListener('gesturestart', function(e) { e.preventDefault(); }, { passive: false });
        document.addEventListener('gesturechange', function(e) { e.preventDefault(); }, { passive: false });
        document.addEventListener('gestureend', function(e) { e.preventDefault(); });
      </script>`;
      let finalHtml = rawHtml;
      if (finalHtml.includes("<head>")) {
        finalHtml = finalHtml.replace("<head>", "<head>" + meta + themeCss + gestureScript);
      } else {
        finalHtml = "<!DOCTYPE html><html><head>" + meta + themeCss + gestureScript + "</head><body>" + finalHtml + "</body></html>";
      }
      frame.srcdoc = finalHtml;
    })
    .catch(err => {
      frame.srcdoc = `<div style="padding:20px;color:#ef4444;font-family:sans-serif;font-size:12px;">加载交互组件失败: ${escapeHtml(err.message)}</div>`;
    });
};

window.refreshAgentEmbedCard = function(cardId) {
  const card = document.getElementById(cardId);
  if (!card) return;
  const src = card.dataset.src;
  if (src && window.loadAgentEmbedContent) {
    window.loadAgentEmbedContent(cardId, src);
  }
};

window.toggleFullscreenAgentEmbed = function(cardId) {
  const card = document.getElementById(cardId);
  if (!card) return;
  card.classList.toggle("fullscreen");
};

window.handleThumbnailLoad = function(img) {
  if (!img) return;
  img.classList.add("loaded");
  const card = img.closest(".image-thumb-card");
  if (!card) return;
  card.classList.add("loaded");

  const nw = img.naturalWidth || 0;
  const nh = img.naturalHeight || 0;
  if (nw > 0 && nh > 0) {
    const dimEl = card.querySelector(".image-thumb-dimensions");
    if (dimEl) dimEl.textContent = `${nw}×${nh}`;
    if (nh / nw >= 1.8) {
      const badgeEl = card.querySelector(".image-thumb-badge");
      if (badgeEl) badgeEl.classList.remove("hidden");
    }
  }
};

window.handleThumbnailError = function(img) {
  if (!img) return;
  // If thumbnail fails, try falling back to original image
  if (!img.dataset.fallback && img.dataset.originalSrc && img.src !== img.dataset.originalSrc) {
    img.dataset.fallback = "1";
    img.src = img.dataset.originalSrc;
    return;
  }
  const card = img.closest(".image-thumb-card");
  if (card) {
    card.classList.remove("loaded");
    card.classList.add("load-error");
  }
};

// 属性值里的 ` * _ ~ 会被后面的行内代码 / 粗斜体 / 删除线正则当成标记，把标签切坏
// （例如链接文字是 `abc` 时 data-md-title 里的反引号会吞掉 ">"）。转成实体后浏览器解码结果不变。
function protectMdAttr(v) {
  return String(v).replace(/[`*_~]/g, (c) => `&#${c.charCodeAt(0)};`);
}

function renderInlineMarkdown(text) {
  if (!text) return "";
  if (text.includes("implementation_plan.md") && !text.includes("[implementation_plan.md]") && !text.includes("](implementation_plan.md)")) {
    text = text.replace(/implementation_plan\.md/g, "[implementation_plan.md](implementation_plan.md)");
  }
  if (text.includes("walkthrough.md") && !text.includes("[walkthrough.md]") && !text.includes("](walkthrough.md)")) {
    text = text.replace(/walkthrough\.md/g, "[walkthrough.md](walkthrough.md)");
  }
  if (text.includes("task.md") && !text.includes("[task.md]") && !text.includes("](task.md)")) {
    text = text.replace(/task\.md/g, "[task.md](task.md)");
  }

  // Normalize HTML whitespace & common named entities
  text = text
    .replace(/&(?:nbsp|#160|ensp|emsp);/gi, "\u00A0")
    .replace(/&(?:mdash|#8212);/gi, "—")
    .replace(/&(?:ndash|#8211);/gi, "–")
    .replace(/&(?:hellip|#8230);/gi, "…")
    .replace(/&(?:rarr|#8594);/gi, "→")
    .replace(/&(?:larr|#8592);/gi, "←")
    .replace(/&(?:harr|#8596);/gi, "↔")
    .replace(/&(?:rArr|#8658);/gi, "⇒")
    .replace(/&(?:lArr|#8656);/gi, "⇐")
    .replace(/&(?:hArr|#8660);/gi, "⇔")
    .replace(/&(?:times|#215);/gi, "×")
    .replace(/&(?:divide|#247);/gi, "÷")
    .replace(/&(?:plusmn|#177);/gi, "±")
    .replace(/&(?:le|#8804);/gi, "≤")
    .replace(/&(?:ge|#8805);/gi, "≥")
    .replace(/&(?:ne|#8800);/gi, "≠")
    .replace(/&(?:approx|#8776);/gi, "≈")
    .replace(/&(?:equiv|#8801);/gi, "≡")
    .replace(/&(?:infin|#8734);/gi, "∞")
    .replace(/&(?:bull|#8226);/gi, "•")
    .replace(/&(?:middot|#183);/gi, "·")
    .replace(/&(?:deg|#176);/gi, "°")
    .replace(/&(?:copy|#169);/gi, "©")
    .replace(/&(?:reg|#174);/gi, "®")
    .replace(/&(?:trade|#8482);/gi, "™");

  // Normalize inline HTML tags outside of inline code spans
  const codeSpans = [];
  text = text.replace(/`[^`]+`/g, (m) => {
    const idx = codeSpans.length;
    codeSpans.push(m);
    return `\uE010CODE${idx}\uE011`;
  });

  text = text
    .replace(/[ \t]*<(?:\/br|br\b[^>]*\/?)>[ \t]*\n?/gi, "___HTML_BR___")
    .replace(/<(?:b|strong)\b[^>]*>([\s\S]*?)<\/(?:b|strong)>/gi, "**$1**")
    .replace(/<(?:i|em)\b[^>]*>([\s\S]*?)<\/(?:i|em)>/gi, "*$1*")
    .replace(/<(?:s|del|strike)\b[^>]*>([\s\S]*?)<\/(?:s|del|strike)>/gi, "~~$1~~")
    .replace(/<mark\b[^>]*>([\s\S]*?)<\/mark>/gi, "==$1==")
    .replace(/<code\b[^>]*>([\s\S]*?)<\/code>/gi, "`$1`")
    .replace(/<kbd\b[^>]*>([\s\S]*?)<\/kbd>/gi, "`[$1]`")
    .replace(/<(?:u|ins)\b[^>]*>([\s\S]*?)<\/(?:u|ins)>/gi, "___HTML_U_OPEN___$1___HTML_U_CLOSE___")
    .replace(/<sup\b[^>]*>([\s\S]*?)<\/sup>/gi, "___HTML_SUP_OPEN___$1___HTML_SUP_CLOSE___")
    .replace(/<sub\b[^>]*>([\s\S]*?)<\/sub>/gi, "___HTML_SUB_OPEN___$1___HTML_SUB_CLOSE___");

  text = text.replace(/\uE010CODE(\d+)\uE011/g, (_, idx) => codeSpans[Number(idx)]);

  let html = escapeHtml(text);
  html = html
    .replace(/___HTML_BR___/g, "<br/>")
    .replace(/___HTML_U_OPEN___/g, "<u>")
    .replace(/___HTML_U_CLOSE___/g, "</u>")
    .replace(/___HTML_SUP_OPEN___/g, "<sup>")
    .replace(/___HTML_SUP_CLOSE___/g, "</sup>")
    .replace(/___HTML_SUB_OPEN___/g, "<sub>")
    .replace(/___HTML_SUB_CLOSE___/g, "</sub>");

  // 1. Linked images: [![alt](thumb)](orig)
  html = html.replace(/\[!\[([^\]]*)\]\(([^)]+)\)\]\(([^)]+)\)/g, (_, alt, thumbUrl, origUrl) => {
    return buildImageThumbnailCard(origUrl, thumbUrl, alt);
  });

  // 2. Standard markdown images: ![alt](url)
  html = html.replace(/!\[([^\]]*)\]\(([^)]+)\)/g, (_, alt, url) => {
    return buildImageThumbnailCard(url, null, alt);
  });

  // 3. MEDIA: path anywhere in text
  html = html.replace(/(?:^|\s|<br\/>)MEDIA:([^\s<"'\n]+)/g, (match, path) => {
    return buildImageThumbnailCard(path, null, "");
  });

  // 4. Markdown links with file icon support (only allow safe URL protocols)
  html = html.replace(/(?<!\!)\[([^\]]+)\]\(([^)]+)\)/g, (_, linkText, url) => {
    const rawUrl = htmlUnescape(url);
    if (!isSafeURL(rawUrl)) return `${linkText}`;
    if (isImageResource(rawUrl)) {
      return buildImageThumbnailCard(rawUrl, null, htmlUnescape(linkText));
    }
    const icon = resolveFileIcon(htmlUnescape(linkText)) || resolveFileIcon(rawUrl);
    const lower = rawUrl.toLowerCase();
    const isDoc = isPreviewableDocumentResource(rawUrl);
    const isMd = !isDoc && (lower.endsWith(".md") || lower.endsWith(".markdown") || lower.includes("/brain/") || lower.includes("implementation_plan") || lower.includes("walkthrough"));
    const isPlan = !isDoc && (lower.includes("implementation_plan") || linkText.toLowerCase().includes("implementation_plan") || lower.includes("walkthrough") || linkText.toLowerCase().includes("walkthrough"));
    const extraClass = isPlan ? " plan-btn-link" : (isMd ? " markdown-file-link" : "");
    const arrowSvg = isPlan ? '<svg class="plan-btn-arrow" width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"></path><polyline points="15 3 21 3 21 9"></polyline><line x1="10" y1="14" x2="21" y2="3"></line></svg>' : '';
    const href = protectMdAttr(escapeHtml(rawUrl));
    const titleAttr = protectMdAttr(escapeHtml(linkText));
    const mdAttrs = isMd ? ` data-md-url="${href}" data-md-title="${titleAttr}"` : "";
    if (icon) {
      return `<a href="${href}" class="file-link${extraClass}"${mdAttrs}><img src="/icons/files/${icon}.svg" class="file-icon" alt="" /><span>${linkText}</span>${arrowSvg}</a>`;
    }
    return `<a href="${href}" class="text-link${extraClass}"${mdAttrs}><span>${linkText}</span>${arrowSvg}</a>`;
  });

  // Inline code (e.g. `foo`)
  html = html.replace(/`([^`]+)`/g, '<code class="inline-code">$1</code>');

  // Bold + Italic, Bold, Italic
  html = html.replace(/\*\*\*((?:[^*]|\*(?!\*\*))+?)\*\*\*/g, '<strong><em>$1</em></strong>');
  html = html.replace(/(?<![A-Za-z0-9_{}])___((?:[^_]|_(?!__))+?)___(?![A-Za-z0-9_{}])/g, '<strong><em>$1</em></strong>');
  html = html.replace(/\*\*((?:[^*]|\*(?!\*))+?)\*\*/g, '<strong>$1</strong>');
  html = html.replace(/(?<![A-Za-z0-9_{}])__((?:[^_]|_(?!_))+?)__(?![A-Za-z0-9_{}])/g, '<strong>$1</strong>');
  html = html.replace(/(?<!\*)\*([^*\n]+?)\*(?!\*)/g, '<em>$1</em>');
  html = html.replace(/(?<![A-Za-z0-9_{}])_([^_\n]+?)_(?![A-Za-z0-9_{}])/g, '<em>$1</em>');

  // Strikethrough & Highlight
  html = html.replace(/~~((?:[^~]|~(?!~))+?)~~/g, '<del>$1</del>');
  html = html.replace(/==((?:[^=]|=(?!=))+?)==/g, '<mark class="md-highlight" style="background: rgba(255, 214, 10, 0.28); color: inherit; padding: 0 3px; border-radius: 3px;">$1</mark>');

  // 5. Agent embed tag: <agent-embed src="..."></agent-embed>
  html = html.replace(/&lt;agent-embed\b[^&>]*?\bsrc=[&quot;']([^&quot;']+)&quot;[^&>]*&gt;(?:\s*&lt;\/agent-embed&gt;)?/gi, (_, src) => {
    return buildAgentEmbedCard(htmlUnescape(src));
  });

  return html;
}

const LANGUAGE_REGISTRY = {
  // JVM & Modern Mobile
  kt: { displayName: "Kotlin", color: "#7F52FF", glyph: "" },
  kts: { displayName: "Kotlin Script", color: "#7F52FF", glyph: "" },
  kotlin: { displayName: "Kotlin", color: "#7F52FF", glyph: "" },
  java: { displayName: "Java", color: "#E76F00", glyph: "" },
  jar: { displayName: "Java Archive", color: "#E76F00", glyph: "" },
  scala: { displayName: "Scala", color: "#DC322F", glyph: "" },
  sc: { displayName: "Scala Script", color: "#DC322F", glyph: "" },
  groovy: { displayName: "Groovy", color: "#4298B8", glyph: "" },
  gvy: { displayName: "Groovy", color: "#4298B8", glyph: "" },
  clj: { displayName: "Clojure", color: "#5881D8", glyph: "" },
  cljs: { displayName: "ClojureScript", color: "#5881D8", glyph: "" },
  cljc: { displayName: "Clojure Common", color: "#5881D8", glyph: "" },
  edn: { displayName: "EDN", color: "#5881D8", glyph: "" },
  swift: { displayName: "Swift", color: "#F05138", glyph: "" },
  dart: { displayName: "Dart", color: "#0175C2", glyph: "" },

  // Python & Data Science
  py: { displayName: "Python", color: "#3776AB", glyph: "" },
  pyw: { displayName: "Python", color: "#3776AB", glyph: "" },
  python: { displayName: "Python", color: "#3776AB", glyph: "" },
  ipynb: { displayName: "Jupyter", color: "#DA5B0B", glyph: "" },
  r: { displayName: "R", color: "#276DC3", glyph: "" },
  rscript: { displayName: "R Script", color: "#276DC3", glyph: "" },
  rmd: { displayName: "R Markdown", color: "#276DC3", glyph: "" },
  julia: { displayName: "Julia", color: "#A270BA", glyph: "" },
  jl: { displayName: "Julia", color: "#A270BA", glyph: "" },

  // JavaScript & TypeScript Ecosystem
  ts: { displayName: "TypeScript", color: "#3178C6", glyph: "" },
  mts: { displayName: "TypeScript Module", color: "#3178C6", glyph: "" },
  cts: { displayName: "TypeScript CommonJS", color: "#3178C6", glyph: "" },
  typescript: { displayName: "TypeScript", color: "#3178C6", glyph: "" },
  tsx: { displayName: "React TSX", color: "#007ACC", glyph: "" },
  js: { displayName: "JavaScript", color: "#F7DF1E", glyph: "" },
  mjs: { displayName: "JavaScript Module", color: "#F7DF1E", glyph: "" },
  cjs: { displayName: "JavaScript CommonJS", color: "#F7DF1E", glyph: "" },
  javascript: { displayName: "JavaScript", color: "#F7DF1E", glyph: "" },
  jsx: { displayName: "React JSX", color: "#61DAFB", glyph: "" },
  vue: { displayName: "Vue", color: "#41B883", glyph: "" },
  svelte: { displayName: "Svelte", color: "#FF3E00", glyph: "" },
  astro: { displayName: "Astro", color: "#FF5D01", glyph: "" },
  mdx: { displayName: "MDX", color: "#1B1F24", glyph: "" },
  marko: { displayName: "Marko", color: "#25D2FF", glyph: "" },
  qml: { displayName: "QML", color: "#41CD52", glyph: "" },

  // Systems & Compiled Languages
  go: { displayName: "Go", color: "#00ADD8", glyph: "" },
  golang: { displayName: "Go", color: "#00ADD8", glyph: "" },
  rs: { displayName: "Rust", color: "#DEA584", glyph: "🦀" },
  rust: { displayName: "Rust", color: "#DEA584", glyph: "🦀" },
  c: { displayName: "C", color: "#555555", glyph: "" },
  h: { displayName: "C Header", color: "#555555", glyph: "" },
  cpp: { displayName: "C++", color: "#00599C", glyph: "" },
  "c++": { displayName: "C++", color: "#00599C", glyph: "" },
  cc: { displayName: "C++", color: "#00599C", glyph: "" },
  cxx: { displayName: "C++", color: "#00599C", glyph: "" },
  hpp: { displayName: "C++ Header", color: "#00599C", glyph: "" },
  hxx: { displayName: "C++ Header", color: "#00599C", glyph: "" },
  cs: { displayName: "C#", color: "#178600", glyph: "" },
  "c#": { displayName: "C#", color: "#178600", glyph: "" },
  csharp: { displayName: "C#", color: "#178600", glyph: "" },
  fs: { displayName: "F#", color: "#B845FC", glyph: "" },
  "f#": { displayName: "F#", color: "#B845FC", glyph: "" },
  fsi: { displayName: "F# Interface", color: "#B845FC", glyph: "" },
  fsx: { displayName: "F# Script", color: "#B845FC", glyph: "" },
  vb: { displayName: "Visual Basic", color: "#945DB7", glyph: "" },
  zig: { displayName: "Zig", color: "#F7A41D", glyph: "" },
  nim: { displayName: "Nim", color: "#FFC200", glyph: "" },
  nims: { displayName: "Nim Script", color: "#FFC200", glyph: "" },
  d: { displayName: "D", color: "#BA595E", glyph: "" },
  di: { displayName: "D Interface", color: "#BA595E", glyph: "" },
  v: { displayName: "V", color: "#4F87C4", glyph: "" },
  vsh: { displayName: "V Script", color: "#4F87C4", glyph: "" },
  odin: { displayName: "Odin", color: "#1389B0", glyph: "" },
  cr: { displayName: "Crystal", color: "#000100", glyph: "" },
  crystal: { displayName: "Crystal", color: "#000100", glyph: "" },
  carbon: { displayName: "Carbon", color: "#117766", glyph: "" },
  m: { displayName: "Objective-C", color: "#438EFF", glyph: "" },
  objc: { displayName: "Objective-C", color: "#438EFF", glyph: "" },
  mm: { displayName: "Objective-C++", color: "#00599C", glyph: "" },
  objcpp: { displayName: "Objective-C++", color: "#00599C", glyph: "" },
  f: { displayName: "Fortran", color: "#4D41B1", glyph: "" },
  for: { displayName: "Fortran", color: "#4D41B1", glyph: "" },
  f90: { displayName: "Fortran 90", color: "#4D41B1", glyph: "" },
  f95: { displayName: "Fortran 95", color: "#4D41B1", glyph: "" },
  fortran: { displayName: "Fortran", color: "#4D41B1", glyph: "" },
  cob: { displayName: "COBOL", color: "#005A9C", glyph: "" },
  cbl: { displayName: "COBOL", color: "#005A9C", glyph: "" },
  cobol: { displayName: "COBOL", color: "#005A9C", glyph: "" },
  pas: { displayName: "Pascal", color: "#E3F171", glyph: "" },
  pp: { displayName: "Pascal", color: "#E3F171", glyph: "" },
  pascal: { displayName: "Pascal", color: "#E3F171", glyph: "" },
  delphi: { displayName: "Delphi", color: "#E3F171", glyph: "" },
  ada: { displayName: "Ada", color: "#02F88C", glyph: "" },
  adb: { displayName: "Ada Body", color: "#02F88C", glyph: "" },
  ads: { displayName: "Ada Spec", color: "#02F88C", glyph: "" },

  // Functional & Scripting Languages
  rb: { displayName: "Ruby", color: "#CC342D", glyph: "💎" },
  ruby: { displayName: "Ruby", color: "#CC342D", glyph: "💎" },
  php: { displayName: "PHP", color: "#4F5D95", glyph: "" },
  lua: { displayName: "Lua", color: "#000080", glyph: "" },
  hs: { displayName: "Haskell", color: "#5E5086", glyph: "" },
  lhs: { displayName: "Haskell", color: "#5E5086", glyph: "" },
  haskell: { displayName: "Haskell", color: "#5E5086", glyph: "" },
  ex: { displayName: "Elixir", color: "#6E4A7E", glyph: "" },
  exs: { displayName: "Elixir Script", color: "#6E4A7E", glyph: "" },
  elixir: { displayName: "Elixir", color: "#6E4A7E", glyph: "" },
  erl: { displayName: "Erlang", color: "#B83998", glyph: "" },
  hrl: { displayName: "Erlang Header", color: "#B83998", glyph: "" },
  erlang: { displayName: "Erlang", color: "#B83998", glyph: "" },
  ocaml: { displayName: "OCaml", color: "#EE6A1A", glyph: "" },
  ml: { displayName: "OCaml", color: "#EE6A1A", glyph: "" },
  mli: { displayName: "OCaml Header", color: "#EE6A1A", glyph: "" },
  elm: { displayName: "Elm", color: "#60B5CC", glyph: "" },
  purs: { displayName: "PureScript", color: "#1D222D", glyph: "" },
  purescript: { displayName: "PureScript", color: "#1D222D", glyph: "" },
  res: { displayName: "ReScript", color: "#E6484F", glyph: "" },
  rescript: { displayName: "ReScript", color: "#E6484F", glyph: "" },
  re: { displayName: "ReasonML", color: "#DB4D3F", glyph: "" },
  sml: { displayName: "Standard ML", color: "#DC5B23", glyph: "" },
  lisp: { displayName: "Lisp", color: "#3FB68B", glyph: "" },
  lsp: { displayName: "Lisp", color: "#3FB68B", glyph: "" },
  el: { displayName: "Emacs Lisp", color: "#7F5AB6", glyph: "" },
  emacs: { displayName: "Emacs Lisp", color: "#7F5AB6", glyph: "" },
  scheme: { displayName: "Scheme", color: "#1E4AEC", glyph: "" },
  scm: { displayName: "Scheme", color: "#1E4AEC", glyph: "" },
  ss: { displayName: "Scheme", color: "#1E4AEC", glyph: "" },
  racket: { displayName: "Racket", color: "#3C5CAA", glyph: "" },
  rkt: { displayName: "Racket", color: "#3C5CAA", glyph: "" },
  pl: { displayName: "Perl", color: "#0298C3", glyph: "" },
  pm: { displayName: "Perl Module", color: "#0298C3", glyph: "" },
  perl: { displayName: "Perl", color: "#0298C3", glyph: "" },
  raku: { displayName: "Raku", color: "#000000", glyph: "" },
  p6: { displayName: "Perl 6", color: "#000000", glyph: "" },

  // Web Styling & Templates
  html: { displayName: "HTML", color: "#E34F26", glyph: "" },
  htm: { displayName: "HTML", color: "#E34F26", glyph: "" },
  css: { displayName: "CSS", color: "#1572B6", glyph: "" },
  scss: { displayName: "SCSS", color: "#CD6799", glyph: "" },
  sass: { displayName: "Sass", color: "#CD6799", glyph: "" },
  less: { displayName: "Less", color: "#1D365D", glyph: "" },
  styl: { displayName: "Stylus", color: "#FF6347", glyph: "" },
  stylus: { displayName: "Stylus", color: "#FF6347", glyph: "" },
  postcss: { displayName: "PostCSS", color: "#DD3A0A", glyph: "" },
  tailwind: { displayName: "Tailwind", color: "#38B2AC", glyph: "" },
  tailwindcss: { displayName: "Tailwind", color: "#38B2AC", glyph: "" },
  pug: { displayName: "Pug", color: "#A86454", glyph: "" },
  jade: { displayName: "Jade", color: "#A86454", glyph: "" },
  hbs: { displayName: "Handlebars", color: "#F7931E", glyph: "" },
  handlebars: { displayName: "Handlebars", color: "#F7931E", glyph: "" },
  mustache: { displayName: "Mustache", color: "#000000", glyph: "" },
  jinja: { displayName: "Jinja", color: "#B41717", glyph: "" },
  jinja2: { displayName: "Jinja2", color: "#B41717", glyph: "" },
  j2: { displayName: "Jinja2", color: "#B41717", glyph: "" },
  twig: { displayName: "Twig", color: "#84B700", glyph: "" },
  liquid: { displayName: "Liquid", color: "#4878FF", glyph: "" },
  blade: { displayName: "Blade", color: "#F05340", glyph: "" },
  erb: { displayName: "ERB", color: "#CC342D", glyph: "" },
  haml: { displayName: "HAML", color: "#ECE2A9", glyph: "" },
  slim: { displayName: "Slim", color: "#2B2B2B", glyph: "" },

  // Shell & System Automation
  sh: { displayName: "Shell", color: "#4EAA25", glyph: "" },
  bash: { displayName: "Bash", color: "#4EAA25", glyph: "" },
  zsh: { displayName: "Zsh", color: "#4EAA25", glyph: "" },
  shell: { displayName: "Shell", color: "#4EAA25", glyph: "" },
  fish: { displayName: "Fish", color: "#D46A4C", glyph: "" },
  ps1: { displayName: "PowerShell", color: "#012456", glyph: "" },
  pwsh: { displayName: "PowerShell", color: "#012456", glyph: "" },
  powershell: { displayName: "PowerShell", color: "#012456", glyph: "" },
  bat: { displayName: "Batch", color: "#C1F12E", glyph: "" },
  cmd: { displayName: "Command", color: "#C1F12E", glyph: "" },
  awk: { displayName: "AWK", color: "#333333", glyph: "" },
  sed: { displayName: "Sed", color: "#4A4A4A", glyph: "" },
  applescript: { displayName: "AppleScript", color: "#1088E0", glyph: "" },
  scpt: { displayName: "AppleScript", color: "#1088E0", glyph: "" },
  ahk: { displayName: "AutoHotkey", color: "#245E1B", glyph: "" },
  autohotkey: { displayName: "AutoHotkey", color: "#245E1B", glyph: "" },
  au3: { displayName: "AutoIt", color: "#0078D7", glyph: "" },
  vim: { displayName: "Vim Script", color: "#019833", glyph: "" },
  viml: { displayName: "Vim Script", color: "#019833", glyph: "" },

  // Shaders, GPU & Hardware
  glsl: { displayName: "GLSL", color: "#5686A5", glyph: "" },
  vert: { displayName: "Vertex Shader", color: "#5686A5", glyph: "" },
  frag: { displayName: "Fragment Shader", color: "#5686A5", glyph: "" },
  geom: { displayName: "Geometry Shader", color: "#5686A5", glyph: "" },
  comp: { displayName: "Compute Shader", color: "#5686A5", glyph: "" },
  hlsl: { displayName: "HLSL", color: "#4382A1", glyph: "" },
  hlsli: { displayName: "HLSL Header", color: "#4382A1", glyph: "" },
  wgsl: { displayName: "WGSL", color: "#4A90E2", glyph: "" },
  metal: { displayName: "Metal Shading", color: "#007AFF", glyph: "" },
  cuda: { displayName: "CUDA", color: "#76B900", glyph: "" },
  cu: { displayName: "CUDA", color: "#76B900", glyph: "" },
  cuh: { displayName: "CUDA Header", color: "#76B900", glyph: "" },
  opencl: { displayName: "OpenCL", color: "#0071C5", glyph: "" },
  verilog: { displayName: "Verilog", color: "#008080", glyph: "" },
  sv: { displayName: "SystemVerilog", color: "#008080", glyph: "" },
  svh: { displayName: "SystemVerilog Header", color: "#008080", glyph: "" },
  systemverilog: { displayName: "SystemVerilog", color: "#008080", glyph: "" },
  vhd: { displayName: "VHDL", color: "#543978", glyph: "" },
  vhdl: { displayName: "VHDL", color: "#543978", glyph: "" },
  bsv: { displayName: "Bluespec", color: "#2980B9", glyph: "" },
  chisel: { displayName: "Chisel", color: "#D32F2F", glyph: "" },
  spice: { displayName: "SPICE", color: "#800080", glyph: "" },
  cir: { displayName: "SPICE Circuit", color: "#800080", glyph: "" },
  gcode: { displayName: "G-Code", color: "#5C6BC0", glyph: "" },

  // Smart Contracts & Web3
  sol: { displayName: "Solidity", color: "#AA6746", glyph: "" },
  solidity: { displayName: "Solidity", color: "#AA6746", glyph: "" },
  vy: { displayName: "Vyper", color: "#2980B9", glyph: "" },
  vyper: { displayName: "Vyper", color: "#2980B9", glyph: "" },
  cairo: { displayName: "Cairo", color: "#E84142", glyph: "" },
  move: { displayName: "Move", color: "#FF5722", glyph: "" },
  clar: { displayName: "Clarity", color: "#5546FF", glyph: "" },
  clarity: { displayName: "Clarity", color: "#5546FF", glyph: "" },
  yul: { displayName: "Yul", color: "#7B1FA2", glyph: "" },
  cdc: { displayName: "Cadence", color: "#00EF8B", glyph: "" },
  cadence: { displayName: "Cadence", color: "#00EF8B", glyph: "" },
  fc: { displayName: "FunC", color: "#0088CC", glyph: "" },
  func: { displayName: "FunC", color: "#0088CC", glyph: "" },
  tact: { displayName: "Tact", color: "#0098EA", glyph: "" },

  // Data, Query & Database
  sql: { displayName: "SQL", color: "#E38C00", glyph: "" },
  mysql: { displayName: "MySQL", color: "#4479A1", glyph: "" },
  pgsql: { displayName: "PostgreSQL", color: "#336791", glyph: "" },
  postgres: { displayName: "PostgreSQL", color: "#336791", glyph: "" },
  postgresql: { displayName: "PostgreSQL", color: "#336791", glyph: "" },
  sqlite: { displayName: "SQLite", color: "#003B57", glyph: "" },
  sqlite3: { displayName: "SQLite", color: "#003B57", glyph: "" },
  tsql: { displayName: "T-SQL", color: "#CC292B", glyph: "" },
  plsql: { displayName: "PL/SQL", color: "#DADADA", glyph: "" },
  redis: { displayName: "Redis", color: "#D82C20", glyph: "" },
  mongo: { displayName: "MongoDB", color: "#13AA52", glyph: "" },
  mongodb: { displayName: "MongoDB", color: "#13AA52", glyph: "" },
  graphql: { displayName: "GraphQL", color: "#E10098", glyph: "" },
  gql: { displayName: "GraphQL", color: "#E10098", glyph: "" },
  sparql: { displayName: "SPARQL", color: "#0C479D", glyph: "" },
  rq: { displayName: "SPARQL", color: "#0C479D", glyph: "" },
  cypher: { displayName: "Cypher", color: "#018BFF", glyph: "" },
  cql: { displayName: "Cypher", color: "#018BFF", glyph: "" },
  prisma: { displayName: "Prisma", color: "#2D3748", glyph: "" },
  promql: { displayName: "PromQL", color: "#E6522C", glyph: "" },
  logql: { displayName: "LogQL", color: "#F46800", glyph: "" },
  kql: { displayName: "Kusto (KQL)", color: "#0078D4", glyph: "" },
  kusto: { displayName: "Kusto", color: "#0078D4", glyph: "" },
  surql: { displayName: "SurrealQL", color: "#FF00A0", glyph: "" },
  edgeql: { displayName: "EdgeQL", color: "#00C853", glyph: "" },

  // Config, Serialization & Declarative Formats
  json: { displayName: "JSON", color: "#CBCB41", glyph: "" },
  jsonc: { displayName: "JSON with Comments", color: "#CBCB41", glyph: "" },
  json5: { displayName: "JSON5", color: "#CBCB41", glyph: "" },
  yaml: { displayName: "YAML", color: "#CB171E", glyph: "" },
  yml: { displayName: "YAML", color: "#CB171E", glyph: "" },
  toml: { displayName: "TOML", color: "#9C4121", glyph: "" },
  xml: { displayName: "XML", color: "#E37933", glyph: "" },
  xsd: { displayName: "XML Schema", color: "#E37933", glyph: "" },
  xsl: { displayName: "XSLT", color: "#E37933", glyph: "" },
  xslt: { displayName: "XSLT", color: "#E37933", glyph: "" },
  ini: { displayName: "INI", color: "#6D8086", glyph: "" },
  cfg: { displayName: "Config", color: "#6D8086", glyph: "" },
  conf: { displayName: "Config", color: "#6D8086", glyph: "" },
  env: { displayName: "ENV", color: "#ECD53F", glyph: "🔑" },
  csv: { displayName: "CSV", color: "#1D6F42", glyph: "" },
  tsv: { displayName: "TSV", color: "#1D6F42", glyph: "" },
  kdl: { displayName: "KDL", color: "#5E81AC", glyph: "" },
  hocon: { displayName: "HOCON", color: "#8F4E8B", glyph: "" },
  ron: { displayName: "RON", color: "#8250DF", glyph: "" },
  dhall: { displayName: "Dhall", color: "#00B4B6", glyph: "" },
  cue: { displayName: "CUE", color: "#00ADD8", glyph: "" },
  jsonnet: { displayName: "Jsonnet", color: "#006494", glyph: "" },
  libsonnet: { displayName: "Jsonnet Lib", color: "#006494", glyph: "" },
  hcl: { displayName: "HCL", color: "#844FBA", glyph: "" },
  tf: { displayName: "Terraform", color: "#844FBA", glyph: "" },
  tfvars: { displayName: "Terraform Vars", color: "#844FBA", glyph: "" },
  terraform: { displayName: "Terraform", color: "#844FBA", glyph: "" },
  bicep: { displayName: "Bicep", color: "#0078D4", glyph: "" },
  nix: { displayName: "Nix", color: "#5277C3", glyph: "" },
  properties: { displayName: "Properties", color: "#2D3748", glyph: "" },

  // DevOps & Build Tooling
  docker: { displayName: "Docker", color: "#2496ED", glyph: "🐳" },
  dockerfile: { displayName: "Dockerfile", color: "#2496ED", glyph: "🐳" },
  cmake: { displayName: "CMake", color: "#064F8C", glyph: "⚙️" },
  meson: { displayName: "Meson", color: "#008B8B", glyph: "" },
  ninja: { displayName: "Ninja", color: "#555555", glyph: "" },
  starlark: { displayName: "Starlark", color: "#43A047", glyph: "" },
  bzl: { displayName: "Bazel Starlark", color: "#43A047", glyph: "" },
  bazel: { displayName: "Bazel", color: "#43A047", glyph: "" },
  just: { displayName: "Justfile", color: "#6C757D", glyph: "⚙️" },
  justfile: { displayName: "Justfile", color: "#6C757D", glyph: "⚙️" },
  earthfile: { displayName: "Earthly", color: "#30B77D", glyph: "🌍" },
  earthly: { displayName: "Earthly", color: "#30B77D", glyph: "🌍" },
  taskfile: { displayName: "Taskfile", color: "#2B5797", glyph: "" },
  vagrant: { displayName: "Vagrantfile", color: "#1563FF", glyph: "📦" },
  procfile: { displayName: "Procfile", color: "#673AB7", glyph: "⚡" },
  bb: { displayName: "BitBake", color: "#2E3440", glyph: "" },
  bbappend: { displayName: "BitBake Append", color: "#2E3440", glyph: "" },

  // Networking, RPC & Protocols
  proto: { displayName: "Protobuf", color: "#4F5D95", glyph: "" },
  protobuf: { displayName: "Protobuf", color: "#4F5D95", glyph: "" },
  capnp: { displayName: "Cap'n Proto", color: "#26A69A", glyph: "" },
  fbs: { displayName: "FlatBuffers", color: "#4DB6AC", glyph: "" },
  flatbuffers: { displayName: "FlatBuffers", color: "#4DB6AC", glyph: "" },
  thrift: { displayName: "Apache Thrift", color: "#F44336", glyph: "" },
  avro: { displayName: "Apache Avro", color: "#00897B", glyph: "" },
  avsc: { displayName: "Avro Schema", color: "#00897B", glyph: "" },
  asn: { displayName: "ASN.1", color: "#78909C", glyph: "" },
  asn1: { displayName: "ASN.1", color: "#78909C", glyph: "" },
  http: { displayName: "HTTP Request", color: "#026E00", glyph: "" },
  rest: { displayName: "REST Client", color: "#026E00", glyph: "" },
  openapi: { displayName: "OpenAPI", color: "#85EA2D", glyph: "" },
  swagger: { displayName: "Swagger", color: "#85EA2D", glyph: "" },
  asyncapi: { displayName: "AsyncAPI", color: "#0885FF", glyph: "" },
  nginx: { displayName: "Nginx", color: "#009639", glyph: "🌐" },
  apache: { displayName: "Apache", color: "#D22128", glyph: "🌐" },
  caddy: { displayName: "Caddyfile", color: "#22B24C", glyph: "🌐" },
  caddyfile: { displayName: "Caddyfile", color: "#22B24C", glyph: "🌐" },
  haproxy: { displayName: "HAProxy", color: "#11557C", glyph: "" },

  // Documentation, Scientific & Markup
  md: { displayName: "Markdown", color: "#4A90E2", glyph: "" },
  markdown: { displayName: "Markdown", color: "#4A90E2", glyph: "" },
  tex: { displayName: "LaTeX", color: "#3D6117", glyph: "" },
  latex: { displayName: "LaTeX", color: "#3D6117", glyph: "" },
  bib: { displayName: "BibTeX", color: "#795548", glyph: "" },
  bibtex: { displayName: "BibTeX", color: "#795548", glyph: "" },
  typ: { displayName: "Typst", color: "#239DAD", glyph: "" },
  typst: { displayName: "Typst", color: "#239DAD", glyph: "" },
  adoc: { displayName: "AsciiDoc", color: "#E40046", glyph: "" },
  asciidoc: { displayName: "AsciiDoc", color: "#E40046", glyph: "" },
  org: { displayName: "Org-mode", color: "#77AA99", glyph: "" },
  rst: { displayName: "reStructuredText", color: "#1480C0", glyph: "" },
  restructuredtext: { displayName: "reStructuredText", color: "#1480C0", glyph: "" },
  textile: { displayName: "Textile", color: "#607D8B", glyph: "" },
  mermaid: { displayName: "Mermaid", color: "#FF3670", glyph: "" },
  puml: { displayName: "PlantUML", color: "#F57C00", glyph: "" },
  plantuml: { displayName: "PlantUML", color: "#F57C00", glyph: "" },
  dot: { displayName: "Graphviz (DOT)", color: "#30638E", glyph: "" },
  gv: { displayName: "Graphviz", color: "#30638E", glyph: "" },

  // Assembly, WebAssembly & Low-Level
  asm: { displayName: "Assembly", color: "#6E4C13", glyph: "" },
  s: { displayName: "Assembly", color: "#6E4C13", glyph: "" },
  assembly: { displayName: "Assembly", color: "#6E4C13", glyph: "" },
  nasm: { displayName: "NASM", color: "#6E4C13", glyph: "" },
  wasm: { displayName: "WebAssembly", color: "#654FF0", glyph: "" },
  wat: { displayName: "WAT", color: "#654FF0", glyph: "" },

  // Diff, Formats, Regex & Grammars
  diff: { displayName: "Diff", color: "#888888", glyph: "" },
  patch: { displayName: "Patch", color: "#888888", glyph: "" },
  txt: { displayName: "Text", color: "#888888", glyph: "" },
  log: { displayName: "Log", color: "#888888", glyph: "" },
  svg: { displayName: "SVG", color: "#FFB13B", glyph: "" },
  regex: { displayName: "RegEx", color: "#009688", glyph: "" },
  regexp: { displayName: "RegEx", color: "#009688", glyph: "" },
  bnf: { displayName: "BNF", color: "#607D8B", glyph: "" },
  ebnf: { displayName: "EBNF", color: "#607D8B", glyph: "" },
  g4: { displayName: "ANTLR", color: "#C0392B", glyph: "" },
  antlr: { displayName: "ANTLR", color: "#C0392B", glyph: "" },
  y: { displayName: "Yacc", color: "#4B6EAF", glyph: "" },
  yacc: { displayName: "Yacc", color: "#4B6EAF", glyph: "" },
  bison: { displayName: "Bison", color: "#4B6EAF", glyph: "" },
  l: { displayName: "Flex", color: "#5C6BC0", glyph: "" },
  flex: { displayName: "Flex", color: "#5C6BC0", glyph: "" },
  lex: { displayName: "Lex", color: "#5C6BC0", glyph: "" },

  // Documents
  pdf: { displayName: "PDF", color: "#F40F02", glyph: "" },
  docx: { displayName: "Word", color: "#185ABD", glyph: "" },
  doc: { displayName: "Word", color: "#185ABD", glyph: "" },
  xlsx: { displayName: "Excel", color: "#1D6F42", glyph: "" },
  xls: { displayName: "Excel", color: "#1D6F42", glyph: "" },
  pptx: { displayName: "PowerPoint", color: "#D24726", glyph: "" },
  ppt: { displayName: "PowerPoint", color: "#D24726", glyph: "" },
  key: { displayName: "Keynote", color: "#007AFF", glyph: "" }
};

function resolveLanguageInfo(lang) {
  if (!lang) return { displayName: "CODE", color: "#9da5b4", glyph: "" };
  const clean = String(lang).trim().toLowerCase().replace(/^`+|`+$/g, "");
  if (!clean) return { displayName: "CODE", color: "#9da5b4", glyph: "" };
  if (LANGUAGE_REGISTRY[clean]) return LANGUAGE_REGISTRY[clean];
  const ext = clean.includes(".") ? clean.split(".").pop() : clean;
  if (LANGUAGE_REGISTRY[ext]) return LANGUAGE_REGISTRY[ext];
  return { displayName: clean.toUpperCase(), color: "#9da5b4", glyph: "" };
}

function renderMarkdown(md) {
  if (!md) return "";
  md = processMathSymbols(md);

  // Plugin card custom tag support (Antigravity 2.31+)
  md = md.replace(/<plugin-card\s+catalog-id=["']([^"']+)["'][^>]*>(?:<\/plugin-card>)?/gi, (match, id) => {
    return `\n\n:::plugin-card ${id}:::\n\n`;
  });

  const lines = md.split("\n");
  const blocks = [];
  let i = 0;

  while (i < lines.length) {
    const line = lines[i];
    const trimmed = line.trim();

    if (!trimmed) {
      i++;
      continue;
    }

    // 0. YAML Frontmatter / Style Block detection at beginning of document
    if (blocks.length === 0) {
      if (trimmed === "---") {
        let endIdx = i + 1;
        let foundEnd = false;
        while (endIdx < lines.length) {
          const t = lines[endIdx].trim();
          if (t === "---" || t === "...") {
            foundEnd = true;
            break;
          }
          endIdx++;
        }
        if (foundEnd && endIdx > i + 1) {
          const fmLines = lines.slice(i + 1, endIdx);
          const lineCount = endIdx - i + 1;
          const codeEscaped = escapeHtml(fmLines.join("\n"));
          blocks.push(`
            <details class="frontmatter-details" style="margin-bottom: 14px; background: rgba(120,120,128,0.08); border-radius: 8px; padding: 7px 12px; font-size: 12px; color: var(--color-text-secondary, #8e8e93);">
              <summary style="cursor: pointer; font-weight: 500; user-select: none; outline: none;">⚙️ 已自动隐藏文档配置与样式 (${lineCount}行)</summary>
              <pre style="margin-top: 8px; font-size: 11px; overflow-x: auto; font-family: ui-monospace, monospace; line-height: 1.4; color: var(--color-text-primary, #1c1c1e); background: rgba(0,0,0,0.03); padding: 8px; border-radius: 6px;"><code>${codeEscaped}</code></pre>
            </details>
          `);
          i = endIdx + 1;
          continue;
        }
      } else if (trimmed.startsWith("marp:") || (trimmed.includes(":") && (trimmed.startsWith("theme:") || trimmed.startsWith("style:")))) {
        let endIdx = i + 1;
        let foundEnd = false;
        while (endIdx < Math.min(lines.length, i + 100)) {
          const t = lines[endIdx].trim();
          if (t === "---") {
            foundEnd = true;
            break;
          }
          if (t.startsWith("# ") || t.startsWith("## ")) {
            break;
          }
          endIdx++;
        }
        if (foundEnd) {
          const fmLines = lines.slice(i, endIdx);
          const lineCount = endIdx - i + 1;
          const codeEscaped = escapeHtml(fmLines.join("\n"));
          blocks.push(`
            <details class="frontmatter-details" style="margin-bottom: 14px; background: rgba(120,120,128,0.08); border-radius: 8px; padding: 7px 12px; font-size: 12px; color: var(--color-text-secondary, #8e8e93);">
              <summary style="cursor: pointer; font-weight: 500; user-select: none; outline: none;">⚙️ 已自动隐藏 Marp 演示配置与样式 (${lineCount}行)</summary>
              <pre style="margin-top: 8px; font-size: 11px; overflow-x: auto; font-family: ui-monospace, monospace; line-height: 1.4; color: var(--color-text-primary, #1c1c1e); background: rgba(0,0,0,0.03); padding: 8px; border-radius: 6px;"><code>${codeEscaped}</code></pre>
            </details>
          `);
          i = endIdx + 1;
          continue;
        }
      }
    }

    // HTML <style>...</style> Block detection
    if (trimmed.startsWith(":::plugin-card")) {
      const match = trimmed.match(/:::plugin-card\s+([^\s:]+)/);
      const pluginId = match ? match[1] : "";
      blocks.push(`
        <div class="plugin-card-view" data-plugin-id="${escapeHtml(pluginId)}">
          <div class="plugin-card-header">
            <span class="plugin-card-title">🧩 推荐插件：${escapeHtml(pluginId)}</span>
            <button class="plugin-card-install-btn" onclick="installPluginFromCard('${escapeHtml(pluginId)}')">一键安装</button>
          </div>
          <div class="plugin-card-desc">来自 Antigravity 官方 / 社区插件市场。安装后自动为您续接对话。</div>
        </div>
      `);
      i++;
      continue;
    }

    if (trimmed.toLowerCase().startsWith("<style")) {
      const styleLines = [];
      let foundEnd = false;
      while (i < lines.length) {
        styleLines.push(lines[i]);
        if (lines[i].toLowerCase().includes("</style>")) {
          foundEnd = true;
          i++;
          break;
        }
        i++;
      }
      if (foundEnd) {
        const codeEscaped = escapeHtml(styleLines.join("\n"));
        blocks.push(`
          <details class="frontmatter-details" style="margin-bottom: 14px; background: rgba(120,120,128,0.08); border-radius: 8px; padding: 7px 12px; font-size: 12px; color: var(--color-text-secondary, #8e8e93);">
            <summary style="cursor: pointer; font-weight: 500; user-select: none; outline: none;">⚙️ 已自动隐藏样式代码 (${styleLines.length}行)</summary>
            <pre style="margin-top: 8px; font-size: 11px; overflow-x: auto; font-family: ui-monospace, monospace; line-height: 1.4; color: var(--color-text-primary, #1c1c1e); background: rgba(0,0,0,0.03); padding: 8px; border-radius: 6px;"><code>${codeEscaped}</code></pre>
          </details>
        `);
        continue;
      }
    }

    // 1. Fenced Code Block or Carousel
    if (trimmed.startsWith("```") || trimmed.startsWith("~~~")) {
      const fenceChar = trimmed[0];
      let fenceCount = 0;
      while (fenceCount < trimmed.length && trimmed[fenceCount] === fenceChar) {
        fenceCount++;
      }
      const fence = fenceChar.repeat(fenceCount);
      const lang = trimmed.slice(fenceCount).trim();
      const codeLines = [];
      i++;
      while (i < lines.length) {
        if (lines[i].trim().startsWith(fence)) {
          i++;
          break;
        }
        codeLines.push(lines[i]);
        i++;
      }
      const rawCode = codeLines.join("\n");
      const langClean = (lang || "").toLowerCase();
      const displayLang = langClean ? langClean.toUpperCase() : "CODE";
      const codeEscaped = escapeHtml(rawCode);

      if (langClean === "carousel") {
        const carouselHtml = renderCarouselBlock(codeLines);
        if (carouselHtml) {
          blocks.push(carouselHtml);
          continue;
        }
      }

      if (langClean === "mermaid") {
        blocks.push(`
          <div class="mermaid-card">
            <div class="mermaid-header">
              <span class="mermaid-title">
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <rect x="3" y="3" width="7" height="7"></rect>
                  <rect x="14" y="3" width="7" height="7"></rect>
                  <rect x="14" y="14" width="7" height="7"></rect>
                  <rect x="3" y="14" width="7" height="7"></rect>
                </svg>
                MERMAID
              </span>
              <div class="mermaid-actions">
                <div class="mermaid-toggle-group">
                  <button class="mermaid-toggle-btn active" data-mode="diagram" onclick="toggleMermaidCard(this, 'diagram')" type="button">图表</button>
                  <button class="mermaid-toggle-btn" data-mode="code" onclick="toggleMermaidCard(this, 'code')" type="button">代码</button>
                </div>
                <button class="code-copy-btn" onclick="copyMermaidCode(this)" type="button" aria-label="复制代码">
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
                    <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
                  </svg>
                  <span>复制</span>
                </button>
              </div>
            </div>
            <div class="mermaid-viewport">
              <div class="mermaid-diagram-wrap">
                <div class="mermaid-render-target" data-processed="false" data-raw-code="${encodeURIComponent(rawCode)}">
                  <span style="font-size:12px;color:var(--ios-tertiary-label);">正在渲染图表...</span>
                </div>
              </div>
              <pre class="mermaid-code-wrap" style="display: none;"><code>${codeEscaped}</code></pre>
            </div>
          </div>
        `);
        continue;
      }

      const langInfo = resolveLanguageInfo(langClean);
      const safeLangClass = String(langClean || "").replace(/[^a-zA-Z0-9_-]/g, "");
      blocks.push(`
        <div class="code-block-card">
          <div class="code-block-header">
            <span class="code-block-lang" style="color: ${escapeHtml(langInfo.color)};">
              <span class="code-block-dot" style="background-color: ${escapeHtml(langInfo.color)};"></span>
              ${langInfo.glyph ? `<span class="code-block-glyph">${langInfo.glyph}</span>` : ""}
              <span>${escapeHtml(langInfo.displayName)}</span>
            </span>
            <button class="code-copy-btn" onclick="copyCode(this)" type="button" aria-label="复制代码">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
                <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
              </svg>
              <span>复制</span>
            </button>
          </div>
          <pre class="code-block-pre"><code class="lang-${escapeHtml(safeLangClass)}">${codeEscaped}</code></pre>
        </div>
      `);
      continue;
    }

    // 2. Horizontal Divider
    if (trimmed === "---" || trimmed === "***" || trimmed === "___") {
      blocks.push(`<hr class="ios-divider" />`);
      i++;
      continue;
    }

    // 3. Headings (# H1..H6)
    if (trimmed.startsWith("#")) {
      let level = 0;
      while (level < trimmed.length && trimmed[level] === "#") {
        level++;
      }
      if (level <= 6 && trimmed.length > level && trimmed[level] === " ") {
        const hText = trimmed.slice(level + 1).trim();
        blocks.push(`<h${level}>${renderInlineMarkdown(hText)}</h${level}>`);
        i++;
        continue;
      }
    }

    // 3.5 HTML <details>...</details> Block
    if (trimmed.toLowerCase().startsWith("<details")) {
      const detailLines = [];
      let foundEnd = false;
      while (i < lines.length) {
        detailLines.push(lines[i]);
        if (lines[i].toLowerCase().includes("</details>")) {
          foundEnd = true;
          i++;
          break;
        }
        i++;
      }
      if (foundEnd) {
        const rawDetails = detailLines.join("\n");
        const summaryMatch = rawDetails.match(/<summary\b[^>]*>([\s\S]*?)<\/summary>/i);
        const summaryText = summaryMatch ? summaryMatch[1].trim() : "详情";
        const bodyRaw = rawDetails
          .replace(/^[\s\S]*?<details\b[^>]*>/i, "")
          .replace(/<summary\b[^>]*>[\s\S]*?<\/summary>/i, "")
          .replace(/<\/details>[\s\S]*$/i, "")
          .trim();
        blocks.push(`
          <details class="md-details" style="margin: 10px 0; background: rgba(120,120,128,0.06); border-radius: 8px; padding: 8px 12px;">
            <summary style="cursor: pointer; font-weight: 600; user-select: none; outline: none;">${renderInlineMarkdown(summaryText)}</summary>
            <div class="md-details-body" style="margin-top: 8px;">${renderMarkdown(bodyRaw)}</div>
          </details>
        `);
        continue;
      }
    }

    // 4. Tables (| Header | Header | or pipe-less GFM Header 1 | Header 2)
    const parseTableRow = (rowStr) => {
      const placeholder = "\uE000";
      const sanitized = rowStr.trim().replace(/\\\|/g, placeholder);
      if (!sanitized.includes("|")) return [];
      let parts = sanitized.split("|");
      if (sanitized.startsWith("|") && parts.length > 0) parts = parts.slice(1);
      if (sanitized.endsWith("|") && parts.length > 0) parts = parts.slice(0, parts.length - 1);
      return parts.map(c => c.replace(/\uE000/g, "|").trim());
    };
    const isTableSepRow = (cells) => cells.length >= 2 && cells.every(cell => /^[\s\-:]+$/.test(cell) && cell.includes("-"));
    const isPipeTableLine = (s) => (s.startsWith("|") && s.endsWith("|") && s.includes("|"));
    const isPipelessTableStart = !isPipeTableLine(trimmed) && trimmed.includes("|") && (i + 1 < lines.length) && isTableSepRow(parseTableRow(lines[i + 1].trim()));

    if (isPipeTableLine(trimmed) || isPipelessTableStart) {
      const tableLines = [];
      while (i < lines.length) {
        const tLine = lines[i].trim();
        if (isPipeTableLine(tLine) || (isPipelessTableStart && tLine.includes("|") && !tLine.startsWith("#") && !tLine.startsWith("```") && !tLine.startsWith(">"))) {
          tableLines.push(tLine);
          i++;
        } else {
          break;
        }
      }
      if (tableLines.length >= 2) {
        const headers = parseTableRow(tableLines[0]);
        let alignments = [];
        const rows = [];
        for (let rIdx = 1; rIdx < tableLines.length; rIdx++) {
          const r = parseTableRow(tableLines[rIdx]);
          // Skip separator row (| --- | :--- |)
          const isSep = r.length > 0 && r.every(cell => /^[\s\-:]+$/.test(cell) && cell.includes("-"));
          if (isSep) {
            if (alignments.length === 0) {
              alignments = r.map(c => {
                const tr = c.trim();
                const left = tr.startsWith(":");
                const right = tr.endsWith(":");
                if (left && right) return "center";
                if (right) return "right";
                return "left";
              });
            }
            continue;
          }
          rows.push(r);
        }

        const maxCols = Math.max(headers.length, ...rows.map(r => r.length));
        if (maxCols > 0) {
          let tableHtml = `<div class="table-wrapper"><table class="ios-markdown-table"><thead><tr>`;
          for (let colIdx = 0; colIdx < maxCols; colIdx++) {
            const h = colIdx < headers.length ? headers[colIdx] : "";
            const align = colIdx < alignments.length ? alignments[colIdx] : "left";
            const alignStyle = align !== "left" ? ` style="text-align:${align};"` : "";
            tableHtml += `<th${alignStyle}>${renderInlineMarkdown(h)}</th>`;
          }
          tableHtml += `</tr></thead><tbody>`;
          for (const row of rows) {
            tableHtml += `<tr>`;
            for (let colIdx = 0; colIdx < maxCols; colIdx++) {
              const cellVal = colIdx < row.length ? row[colIdx] : "";
              const align = colIdx < alignments.length ? alignments[colIdx] : "left";
              const alignStyle = align !== "left" ? ` style="text-align:${align};"` : "";
              tableHtml += `<td${alignStyle}>${renderInlineMarkdown(cellVal)}</td>`;
            }
            tableHtml += `</tr>`;
          }
          tableHtml += `</tbody></table></div>`;
          blocks.push(tableHtml);
          continue;
        }
      }
    }

    // 5. Blockquotes & GitHub Alerts
    if (trimmed.startsWith(">")) {
      const quoteLines = [];
      while (i < lines.length) {
        const qLine = lines[i].trim();
        if (qLine.startsWith(">")) {
          quoteLines.push(qLine.replace(/^>\s?/, ""));
          i++;
        } else {
          break;
        }
      }
      const quoteText = quoteLines.join("\n");
      const alertMatch = quoteText.match(/^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]\s*([\s\S]*)$/i);
      if (alertMatch) {
        const alertType = alertMatch[1].toLowerCase();
        const alertContent = alertMatch[2].trim();
        blocks.push(`
          <div class="ios-alert alert-${alertType}">
            <div class="alert-title">${alertMatch[1].toUpperCase()}</div>
            <div class="alert-content">${renderInlineMarkdown(alertContent).replace(/\n/g, "<br/>")}</div>
          </div>
        `);
      } else {
        blocks.push(`<blockquote class="ios-blockquote">${renderInlineMarkdown(quoteText).replace(/\n/g, "<br/>")}</blockquote>`);
      }
      continue;
    }

    // 6. Lists (Unordered & Ordered, with Task List checkbox support)
    const isUnordered = trimmed.startsWith("- ") || trimmed.startsWith("* ") || trimmed.startsWith("+ ") || trimmed.startsWith("• ");
    const orderedMatch = trimmed.match(/^(\d+)[.)]\s+/);
    const isOrdered = Boolean(orderedMatch);
    if (isUnordered || isOrdered) {
      const listItems = [];
      const tag = isOrdered ? "ol" : "ul";
      const startNum = orderedMatch ? parseInt(orderedMatch[1], 10) : 1;
      while (i < lines.length) {
        const lLine = lines[i].trim();
        if (isOrdered && /^\d+[.)]\s+/.test(lLine)) {
          listItems.push(lLine.replace(/^\d+[.)]\s+/, ""));
          i++;
        } else if (!isOrdered && (lLine.startsWith("- ") || lLine.startsWith("* ") || lLine.startsWith("+ ") || lLine.startsWith("• "))) {
          listItems.push(lLine.replace(/^[-*+•]\s+/, ""));
          i++;
        } else {
          break;
        }
      }
      const renderListItem = (it) => {
        let prefix = "";
        let rest = it;
        if (/^\[ \]\s+/.test(rest)) {
          prefix = `<span class="md-task-checkbox" style="margin-right: 6px; user-select: none;">☐</span>`;
          rest = rest.replace(/^\[ \]\s+/, "");
        } else if (/^\[[xX]\]\s+/.test(rest)) {
          prefix = `<span class="md-task-checkbox checked" style="margin-right: 6px; user-select: none;">☑</span>`;
          rest = rest.replace(/^\[[xX]\]\s+/, "");
        } else if (/^\[[\/\-]\]\s+/.test(rest)) {
          prefix = `<span class="md-task-checkbox partial" style="margin-right: 6px; user-select: none;">◐</span>`;
          rest = rest.replace(/^\[[\/\-]\]\s+/, "");
        }
        return `<li>${prefix}${renderInlineMarkdown(rest)}</li>`;
      };
      const itemsHtml = listItems.map(renderListItem).join("");
      const startAttr = (isOrdered && startNum > 1) ? ` start="${startNum}"` : "";
      blocks.push(`<${tag} class="ios-list"${startAttr}>${itemsHtml}</${tag}>`);
      continue;
    }

    // 6.5 Standalone MEDIA: or Image Block
    if (trimmed.startsWith("MEDIA:") || /^(?:https?:\/\/[^\s]+\.(?:png|jpe?g|webp|gif|svg|bmp)|(?:\/|[a-zA-Z]:\\|file:\/\/)[^\s<"']+\.(?:png|jpe?g|webp|gif|svg|bmp))$/i.test(trimmed)) {
      const imgPath = trimmed.startsWith("MEDIA:") ? trimmed.slice(6).trim() : trimmed;
      blocks.push(buildImageThumbnailCard(imgPath, null, ""));
      i++;
      continue;
    }

    // 6.6 Standalone Agent Embed Block
    const standaloneEmbedMatch = trimmed.match(/^<agent-embed\b[^>]*?\bsrc=["']([^"']+)["'][^>]*>(?:\s*<\/agent-embed>)?$/i);
    if (standaloneEmbedMatch) {
      blocks.push(buildAgentEmbedCard(standaloneEmbedMatch[1]));
      i++;
      continue;
    }

    // 7. Paragraph
    const paraLines = [line];
    i++;
    while (i < lines.length) {
      const nextLine = lines[i];
      const nTrimmed = nextLine.trim();
      const nPipelessTable = !isPipeTableLine(nTrimmed) && nTrimmed.includes("|") && (i + 1 < lines.length) && isTableSepRow(parseTableRow(lines[i + 1].trim()));
      if (!nTrimmed ||
          nTrimmed.startsWith("MEDIA:") ||
          /^<agent-embed\b/i.test(nTrimmed) ||
          nTrimmed.toLowerCase().startsWith("<details") ||
          /^(?:https?:\/\/[^\s]+\.(?:png|jpe?g|webp|gif|svg|bmp)|(?:\/|[a-zA-Z]:\\|file:\/\/)[^\s<"']+\.(?:png|jpe?g|webp|gif|svg|bmp))$/i.test(nTrimmed) ||
          nTrimmed.startsWith("```") ||
          nTrimmed.startsWith("~~~") ||
          nTrimmed.startsWith("#") ||
          nTrimmed === "---" || nTrimmed === "***" || nTrimmed === "___" ||
          (nTrimmed.startsWith("|") && nTrimmed.endsWith("|")) ||
          nPipelessTable ||
          nTrimmed.startsWith(">") ||
          nTrimmed.startsWith("- ") || nTrimmed.startsWith("* ") || nTrimmed.startsWith("+ ") || nTrimmed.startsWith("• ") ||
          /^\d+[.)]\s/.test(nTrimmed)) {
        break;
      }
      paraLines.push(nextLine);
      i++;
    }
    const paraHtml = renderInlineMarkdown(paraLines.join("\n")).replace(/\n/g, "<br/>");
    blocks.push(`<p>${paraHtml}</p>`);
  }

  return blocks.join("");
}

window.copyCode = function(btn) {
  const card = btn.closest(".code-block-card");
  if (!card) return;
  const codeEl = card.querySelector("code");
  if (!codeEl) return;
  const text = codeEl.innerText;
  navigator.clipboard.writeText(text).then(() => {
    const span = btn.querySelector("span");
    if (span) {
      const orig = span.textContent;
      span.textContent = "已复制";
      btn.classList.add("copied");
      setTimeout(() => {
        span.textContent = orig;
        btn.classList.remove("copied");
      }, 1500);
    }
  }).catch(() => {});
};

window.copyMermaidCode = function(btn) {
  const card = btn.closest(".mermaid-card");
  if (!card) return;
  const codeEl = card.querySelector(".mermaid-code-wrap code");
  if (!codeEl) return;
  const text = codeEl.innerText;
  navigator.clipboard.writeText(text).then(() => {
    const span = btn.querySelector("span");
    if (span) {
      const orig = span.textContent;
      span.textContent = "已复制";
      btn.classList.add("copied");
      setTimeout(() => {
        span.textContent = orig;
        btn.classList.remove("copied");
      }, 1500);
    }
  }).catch(() => {});
};

window.toggleMermaidCard = function(btn, mode) {
  const card = btn.closest(".mermaid-card");
  if (!card) return;
  const toggleBtns = card.querySelectorAll(".mermaid-toggle-btn");
  toggleBtns.forEach(b => b.classList.remove("active"));
  btn.classList.add("active");

  const diagWrap = card.querySelector(".mermaid-diagram-wrap");
  const codeWrap = card.querySelector(".mermaid-code-wrap");

  if (mode === "code") {
    if (diagWrap) diagWrap.style.display = "none";
    if (codeWrap) codeWrap.style.display = "block";
  } else {
    if (diagWrap) diagWrap.style.display = "flex";
    if (codeWrap) codeWrap.style.display = "none";
  }
};

// --- Interactive Photo Carousel Helpers ---
function renderCarouselBlock(lines) {
  const slides = [];
  let curLines = [];
  let curTitle = null;
  const slideRegex = /^<!--\s*slide(?::\s*([^>]*))?\s*-->$/i;

  for (const line of lines) {
    const trimmed = line.trim();
    const match = trimmed.match(slideRegex);
    if (match) {
      const joined = curLines.join("\n").trim();
      if (joined) {
        slides.push({ title: curTitle, content: joined });
      }
      curLines = [];
      curTitle = match[1] ? match[1].trim() : null;
    } else {
      curLines.push(line);
    }
  }
  const lastJoined = curLines.join("\n").trim();
  if (lastJoined) {
    slides.push({ title: curTitle, content: lastJoined });
  }

  if (slides.length === 0) return "";

  const carouselId = "carousel-" + Math.random().toString(36).slice(2, 9);
  const total = slides.length;

  const slidesHtml = slides.map((s, idx) => {
    return `<div class="carousel-slide-pane ${idx === 0 ? "active" : ""}" data-index="${idx}">
      ${renderMarkdown(s.content)}
    </div>`;
  }).join("");

  const dotsHtml = total > 1 ? `
    <div class="carousel-dots-row">
      ${slides.map((_, idx) => `
        <span class="carousel-dot ${idx === 0 ? "active" : ""}" data-index="${idx}" onclick="jumpCarouselDot(this, ${idx})"></span>
      `).join("")}
    </div>
  ` : "";

  return `
    <div class="carousel-card" id="${carouselId}" data-current="0" data-total="${total}">
      <div class="carousel-header">
        <div class="carousel-title-group">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="carousel-icon">
            <rect x="3" y="3" width="18" height="18" rx="2" ry="2"></rect>
            <circle cx="8.5" cy="8.5" r="1.5"></circle>
            <polyline points="21 15 16 10 5 21"></polyline>
          </svg>
          <span class="carousel-title">照片轮播</span>
          <span class="carousel-counter-badge"><span class="carousel-current-num">1</span> / ${total}</span>
        </div>
        <div class="carousel-nav-buttons">
          <button class="carousel-nav-btn btn-prev" disabled onclick="navigateCarousel(this, -1)" type="button" aria-label="上一张">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="15 18 9 12 15 6"></polyline>
            </svg>
            <span>上一张</span>
          </button>
          <button class="carousel-nav-btn btn-next" ${total <= 1 ? "disabled" : ""} onclick="navigateCarousel(this, 1)" type="button" aria-label="下一张">
            <span>下一张</span>
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="9 18 15 12 9 6"></polyline>
            </svg>
          </button>
        </div>
      </div>
      <div class="carousel-viewport">
        ${slidesHtml}
      </div>
      ${dotsHtml}
    </div>
  `;
}

window.navigateCarousel = function(btn, dir) {
  const card = btn.closest(".carousel-card");
  if (!card) return;
  const current = parseInt(card.getAttribute("data-current") || "0", 10);
  const total = parseInt(card.getAttribute("data-total") || "1", 10);
  const next = current + dir;
  if (next < 0 || next >= total) return;
  setCarouselIndex(card, next);
};

window.jumpCarouselDot = function(dot, targetIdx) {
  const card = dot.closest(".carousel-card");
  if (!card) return;
  setCarouselIndex(card, targetIdx);
};

function setCarouselIndex(card, targetIdx) {
  const total = parseInt(card.getAttribute("data-total") || "1", 10);
  if (targetIdx < 0 || targetIdx >= total) return;
  card.setAttribute("data-current", targetIdx.toString());

  const counterEl = card.querySelector(".carousel-current-num");
  if (counterEl) counterEl.textContent = (targetIdx + 1).toString();

  const prevBtn = card.querySelector(".carousel-nav-btn.btn-prev");
  const nextBtn = card.querySelector(".carousel-nav-btn.btn-next");
  if (prevBtn) prevBtn.disabled = (targetIdx === 0);
  if (nextBtn) nextBtn.disabled = (targetIdx >= total - 1);

  const slides = card.querySelectorAll(".carousel-slide-pane");
  slides.forEach((s, idx) => {
    if (idx === targetIdx) {
      s.classList.add("active");
    } else {
      s.classList.remove("active");
    }
  });

  const dots = card.querySelectorAll(".carousel-dot");
  dots.forEach((d, idx) => {
    if (idx === targetIdx) {
      d.classList.add("active");
    } else {
      d.classList.remove("active");
    }
  });
}
window.setCarouselIndex = setCarouselIndex;

let mermaidInitialized = false;
function initMermaidIfNeeded() {
  if (typeof mermaid === "undefined") return false;
  if (!mermaidInitialized) {
    const isDark = !(window.matchMedia && window.matchMedia("(prefers-color-scheme: light)").matches);
    try {
      mermaid.initialize({
        startOnLoad: false,
        theme: isDark ? "dark" : "default",
        securityLevel: "strict",
        fontFamily: "-apple-system, BlinkMacSystemFont, 'SF Pro Display', 'SF Pro Text', 'PingFang SC', sans-serif"
      });
      mermaidInitialized = true;
    } catch (e) {
      console.warn("Failed to initialize mermaid:", e);
    }
  }
  return mermaidInitialized;
}

function sanitizeSVG(svgStr) {
  if (!svgStr) return "";
  return svgStr
    .replace(/<script\b[^<]*(?:(?!<\/script>)<[^<]*)*<\/script>/gi, "")
    .replace(/\son\w+\s*=\s*(['"]).*?\1/gi, "")
    .replace(/\son\w+\s*=\s*[^>\s]+/gi, "")
    .replace(/href\s*=\s*(['"])javascript:.*?\1/gi, 'href="#"');
}

let mermaidLoadPromise = null;
function loadMermaidScript() {
  if (typeof mermaid !== "undefined") return Promise.resolve();
  if (mermaidLoadPromise) return mermaidLoadPromise;
  mermaidLoadPromise = new Promise((resolve, reject) => {
    const script = document.createElement("script");
    script.src = "/mermaid.min.js?v=1";
    script.async = true;
    script.onload = () => resolve();
    script.onerror = (e) => {
      mermaidLoadPromise = null;
      reject(e);
    };
    document.head.appendChild(script);
  });
  return mermaidLoadPromise;
}

function repairMermaid(code) {
  if (!code || typeof code !== 'string') return code;
  try {
    const lines = code.split('\n');
    const repairedLines = lines.map(function(line) {
      let l = line;
      const trimmed = l.trim();
      if (trimmed.indexOf('%%') === 0 || trimmed.indexOf('graph ') === 0 || trimmed.indexOf('flowchart ') === 0 || trimmed.indexOf('sequenceDiagram') === 0 || trimmed.indexOf('classDiagram') === 0 || trimmed.indexOf('stateDiagram') === 0 || trimmed.indexOf('erDiagram') === 0) {
        return l;
      }

      const stashedStrings = [];
      l = l.replace(/"([^"\\]*(?:\\.[^"\\]*)*)"/g, function(match) {
        const token = '___STR_' + stashedStrings.length + '___';
        stashedStrings.push(match);
        return token;
      });

      // Subgraph with unquoted sensitive characters
      l = l.replace(/^(\s*subgraph\s+)([A-Za-z0-9_\-\u4e00-\u9fa5]+)\s*\[([^\[\]\r\n]+)\](?=[^\]]|$)/g, function(match, prefix, id, content) {
        const cTrim = content.trim();
        if (cTrim.indexOf('___STR_') === 0 && cTrim.slice(-3) === '___') return match;
        if (/[()[\]{}+*#;:]/.test(cTrim)) {
          const safe = cTrim.replace(/"/g, "'");
          const token = '___STR_' + stashedStrings.length + '___';
          stashedStrings.push('"' + safe + '"');
          return prefix + id + '[' + token + ']';
        }
        return match;
      });

      // Double curly braces (Hexagon): ID{{content}}
      l = l.replace(/(^|[>\s\-|&,;])([A-Za-z0-9_\-\u4e00-\u9fa5]+)\s*\{\{([^{}\r\n]+)\}\}(?=[^}]|$)/g, function(match, prefix, id, content) {
        const cTrim = content.trim();
        if (cTrim.indexOf('___STR_') === 0 && cTrim.slice(-3) === '___') return match;
        if (/[()[\]{}+*#;:]/.test(cTrim)) {
          const safe = cTrim.replace(/"/g, "'");
          const token = '___STR_' + stashedStrings.length + '___';
          stashedStrings.push('"' + safe + '"');
          return prefix + id + '{{' + token + '}}';
        }
        return match;
      });

      // Single curly braces (Rhombus/Diamond): ID{content}
      l = l.replace(/(^|[>\s\-|&,;])([A-Za-z0-9_\-\u4e00-\u9fa5]+)\s*\{([^{}\r\n]+)\}(?=[^}]|$)/g, function(match, prefix, id, content) {
        const cTrim = content.trim();
        if (cTrim.indexOf('___STR_') === 0 && cTrim.slice(-3) === '___') return match;
        if (/[()[\]{}+*#;:]/.test(cTrim)) {
          const safe = cTrim.replace(/"/g, "'");
          const token = '___STR_' + stashedStrings.length + '___';
          stashedStrings.push('"' + safe + '"');
          return prefix + id + '{' + token + '}';
        }
        return match;
      });

      // Square brackets: ID[content]
      l = l.replace(/(^|[>\s\-|&,;])([A-Za-z0-9_\-\u4e00-\u9fa5]+)\s*\[([^\[\]\r\n]+)\](?=[^\]]|$)/g, function(match, prefix, id, content) {
        const cTrim = content.trim();
        if (cTrim.indexOf('___STR_') === 0 && cTrim.slice(-3) === '___') return match;
        if (cTrim.indexOf('(') === 0 && cTrim.slice(-1) === ')') return match;
        if (/[()[\]{}+*#;:]/.test(cTrim)) {
          const safe = cTrim.replace(/"/g, "'");
          const token = '___STR_' + stashedStrings.length + '___';
          stashedStrings.push('"' + safe + '"');
          return prefix + id + '[' + token + ']';
        }
        return match;
      });

      // Round parentheses: ID(content)
      l = l.replace(/(^|[>\s\-|&,;])([A-Za-z0-9_\-\u4e00-\u9fa5]+)\s*\(([^()\r\n]+)\)(?=[^)]|$)/g, function(match, prefix, id, content) {
        const cTrim = content.trim();
        if (cTrim.indexOf('___STR_') === 0 && cTrim.slice(-3) === '___') return match;
        if (cTrim.indexOf('[') === 0 && cTrim.slice(-1) === ']') return match;
        if (/[()[\]{}+*#;:]/.test(cTrim)) {
          const safe = cTrim.replace(/"/g, "'");
          const token = '___STR_' + stashedStrings.length + '___';
          stashedStrings.push('"' + safe + '"');
          return prefix + id + '(' + token + ')';
        }
        return match;
      });

      // Restore stashed strings
      for (let i = 0; i < stashedStrings.length; i++) {
        l = l.split('___STR_' + i + '___').join(stashedStrings[i]);
      }

      return l;
    });

    return repairedLines.join('\n');
  } catch (e) {
    return code;
  }
}

let mermaidRenderCounter = 0;
async function renderAllMermaidDiagrams(root = document) {
  const targets = (root && root.querySelectorAll) ? root.querySelectorAll('.mermaid-render-target[data-processed="false"]') : [];
  if (!targets || targets.length === 0) return;

  if (typeof mermaid === "undefined") {
    try {
      await loadMermaidScript();
    } catch (e) {
      console.warn("Failed to load mermaid.min.js on demand:", e);
      return;
    }
  }
  initMermaidIfNeeded();

  for (const target of targets) {
    target.setAttribute("data-processed", "true");
    const rawEncoded = target.getAttribute("data-raw-code") || "";
    let code = "";
    try {
      code = decodeURIComponent(rawEncoded);
    } catch {
      code = rawEncoded;
    }

    const uniqueId = `mermaid-svg-${Date.now()}-${++mermaidRenderCounter}`;
    try {
      let result;
      try {
        result = await mermaid.render(uniqueId, code);
      } catch (renderErr) {
        const repaired = repairMermaid(code);
        if (repaired && repaired !== code) {
          const retryId = `mermaid-svg-${Date.now()}-${++mermaidRenderCounter}`;
          result = await mermaid.render(retryId, repaired);
        } else {
          throw renderErr;
        }
      }

      if (!target.isConnected) return;
      target.innerHTML = sanitizeSVG(result.svg);
      if (typeof result.bindFunctions === "function") {
        result.bindFunctions(target);
      }
    } catch (err) {
      if (!target.isConnected) return;
      console.warn("Mermaid render error:", err);
      const tempErr = document.getElementById("d" + uniqueId);
      if (tempErr) tempErr.remove();
      const bodySvgs = document.querySelectorAll(`body > svg[id="${uniqueId}"], body > svg#d${uniqueId}`);
      bodySvgs.forEach(s => s.remove());

      target.innerHTML = `
        <div class="mermaid-error">
          <div style="font-weight:600;margin-bottom:4px;">图表解析错误</div>
          <div style="font-size:11px;opacity:0.85;">${escapeHtml(err.message || String(err))}</div>
        </div>
      `;
    }
  }
}
window.renderAllMermaidDiagrams = renderAllMermaidDiagrams;

// Markdown & LaTeX Parsing Memory Cache (LRU)
const markdownCache = new Map();
/** FNV-1a hash for fast full-text cache key generation */
function fnv1aHash(str) {
  let hash = 0x811c9dc5;
  for (let i = 0; i < str.length; i++) {
    hash ^= str.charCodeAt(i);
    hash = (hash * 0x01000193) >>> 0;
  }
  return hash.toString(36);
}
function getCachedMarkdown(md) {
  if (!md) return "";
  const key = fnv1aHash(md);
  if (markdownCache.has(key)) {
    return markdownCache.get(key);
  }
  const html = renderMarkdown(md);
  if (markdownCache.size > 1000) {
    const firstKey = markdownCache.keys().next().value;
    markdownCache.delete(firstKey);
  }
  markdownCache.set(key, html);
  return html;
}

// --- Full-Screen Image Lightbox Viewer Manager ---
const ImageViewerManager = {
  currentUrl: "",
  currentTitle: "",
  currentScale: 1.0,
  minScale: 0.3,
  maxScale: 6.0,
  isDragging: false,
  dragStartX: 0,
  dragStartY: 0,
  translateX: 0,
  translateY: 0,
  initialPinchDist: null,
  initialPinchScale: 1.0,
  isLongScreenshot: false,
  naturalWidth: 0,
  naturalHeight: 0,

  init() {
    const modal = document.getElementById("image-viewer-modal");
    if (!modal || modal.dataset.initialized) return;
    modal.dataset.initialized = "true";

    // Close button & backdrop
    document.getElementById("btn-image-viewer-close")?.addEventListener("click", () => this.close());
    document.getElementById("image-viewer-backdrop")?.addEventListener("click", (e) => {
      if (e.target === e.currentTarget) this.close();
    });

    // Action buttons
    document.getElementById("btn-image-viewer-download")?.addEventListener("click", () => this.downloadImage());
    document.getElementById("btn-image-viewer-external")?.addEventListener("click", () => this.openExternal());

    // Zoom buttons
    document.getElementById("btn-image-zoom-in")?.addEventListener("click", () => this.zoom(0.3));
    document.getElementById("btn-image-zoom-out")?.addEventListener("click", () => this.zoom(-0.3));
    document.getElementById("btn-image-zoom-fit")?.addEventListener("click", () => this.zoomFit());
    document.getElementById("btn-image-zoom-actual")?.addEventListener("click", () => this.zoomActual());

    // Keyboard navigation (Esc to close)
    document.addEventListener("keydown", (e) => {
      if (e.key === "Escape" && !modal.classList.contains("hidden")) {
        this.close();
      }
    });

    // Viewport mouse wheel zoom & drag
    const viewport = document.getElementById("image-viewer-viewport");
    const img = document.getElementById("image-viewer-img");

    if (viewport && img) {
      viewport.addEventListener("wheel", (e) => {
        if (modal.classList.contains("hidden")) return;
        e.preventDefault();
        const delta = e.deltaY < 0 ? 0.2 : -0.2;
        this.zoom(delta);
      }, { passive: false });

      // Double-click to toggle fit / 2.5x
      img.addEventListener("dblclick", (e) => {
        e.preventDefault();
        if (this.currentScale > 1.1) {
          this.zoomFit();
        } else {
          this.zoomTo(2.5);
        }
      });

      // Mouse drag panning
      viewport.addEventListener("mousedown", (e) => {
        if (modal.classList.contains("hidden") || e.button !== 0) return;
        if (this.currentScale <= 1.05 && !this.isLongScreenshot) return;
        this.isDragging = true;
        this.dragStartX = e.clientX - this.translateX;
        this.dragStartY = e.clientY - this.translateY;
        viewport.style.cursor = "grabbing";
      });

      window.addEventListener("mousemove", (e) => {
        if (!this.isDragging) return;
        this.translateX = e.clientX - this.dragStartX;
        this.translateY = e.clientY - this.dragStartY;
        this.applyTransform();
      });

      window.addEventListener("mouseup", () => {
        if (this.isDragging) {
          this.isDragging = false;
          if (viewport) viewport.style.cursor = "";
        }
      });

      // Touch gestures: Pinch-to-zoom & Double-tap
      let lastTapTime = 0;
      viewport.addEventListener("touchstart", (e) => {
        if (modal.classList.contains("hidden")) return;
        if (e.touches.length === 2) {
          this.initialPinchDist = Math.hypot(
            e.touches[0].clientX - e.touches[1].clientX,
            e.touches[0].clientY - e.touches[1].clientY
          );
          this.initialPinchScale = this.currentScale;
        } else if (e.touches.length === 1) {
          const now = Date.now();
          if (now - lastTapTime < 300) {
            // Double-tap
            e.preventDefault();
            if (this.currentScale > 1.1) {
              this.zoomFit();
            } else {
              this.zoomTo(2.5);
            }
          }
          lastTapTime = now;
          if (this.currentScale > 1.05) {
            this.isDragging = true;
            this.dragStartX = e.touches[0].clientX - this.translateX;
            this.dragStartY = e.touches[0].clientY - this.translateY;
          }
        }
      }, { passive: false });

      viewport.addEventListener("touchmove", (e) => {
        if (modal.classList.contains("hidden")) return;
        if (e.touches.length === 2 && this.initialPinchDist) {
          e.preventDefault();
          const dist = Math.hypot(
            e.touches[0].clientX - e.touches[1].clientX,
            e.touches[0].clientY - e.touches[1].clientY
          );
          const ratio = dist / this.initialPinchDist;
          this.zoomTo(this.initialPinchScale * ratio);
        } else if (e.touches.length === 1 && this.isDragging) {
          e.preventDefault();
          this.translateX = e.touches[0].clientX - this.dragStartX;
          this.translateY = e.touches[0].clientY - this.dragStartY;
          this.applyTransform();
        }
      }, { passive: false });

      viewport.addEventListener("touchend", (e) => {
        if (e.touches.length < 2) {
          this.initialPinchDist = null;
        }
        if (e.touches.length === 0) {
          this.isDragging = false;
        }
      });
    }
  },

  open(originalUrl, title, thumbUrl) {
    this.init();
    const modal = document.getElementById("image-viewer-modal");
    if (!modal) return;

    this.currentUrl = originalUrl;
    this.currentTitle = title || "原图预览";
    this.currentScale = 1.0;
    this.translateX = 0;
    this.translateY = 0;

    const titleEl = document.getElementById("image-viewer-filename");
    const metaEl = document.getElementById("image-viewer-meta");
    const loadingEl = document.getElementById("image-viewer-loading");
    const img = document.getElementById("image-viewer-img");
    const viewport = document.getElementById("image-viewer-viewport");

    if (titleEl) titleEl.textContent = this.currentTitle;
    if (metaEl) metaEl.textContent = "正在载入高清原图...";
    if (loadingEl) loadingEl.classList.remove("hidden");
    if (viewport) viewport.scrollTop = 0;

    if (img) {
      img.classList.remove("is-long-screenshot");
      img.style.transform = "";
      img.src = "";

      const tempImg = new Image();
      tempImg.onload = () => {
        this.naturalWidth = tempImg.naturalWidth;
        this.naturalHeight = tempImg.naturalHeight;
        this.isLongScreenshot = (this.naturalHeight / this.naturalWidth) >= 1.8;

        img.src = this.currentUrl;
        if (this.isLongScreenshot) {
          img.classList.add("is-long-screenshot");
        }
        if (loadingEl) loadingEl.classList.add("hidden");
        if (metaEl) {
          metaEl.textContent = `${this.naturalWidth} × ${this.naturalHeight} px${this.isLongScreenshot ? " · 高清长图" : ""}`;
        }
        this.zoomFit();
      };
      tempImg.onerror = () => {
        if (loadingEl) loadingEl.classList.add("hidden");
        if (metaEl) metaEl.textContent = "图片加载失败";
        if (thumbUrl && thumbUrl !== this.currentUrl) {
          img.src = thumbUrl;
        }
      };
      tempImg.src = this.currentUrl;
    }

    modal.classList.remove("hidden");
    triggerHaptic("selection");
  },

  close() {
    const modal = document.getElementById("image-viewer-modal");
    if (modal) modal.classList.add("hidden");
    const img = document.getElementById("image-viewer-img");
    if (img) {
      img.src = "";
      img.style.transform = "";
    }
  },

  zoom(delta) {
    this.zoomTo(this.currentScale + delta);
  },

  zoomTo(scale) {
    this.currentScale = Math.max(this.minScale, Math.min(this.maxScale, scale));
    this.applyTransform();
  },

  zoomFit() {
    this.currentScale = 1.0;
    this.translateX = 0;
    this.translateY = 0;
    this.applyTransform();
    const viewport = document.getElementById("image-viewer-viewport");
    if (viewport) viewport.scrollTop = 0;
  },

  zoomActual() {
    this.currentScale = 1.0;
    this.translateX = 0;
    this.translateY = 0;
    const img = document.getElementById("image-viewer-img");
    if (img && this.naturalWidth > 0) {
      const containerWidth = img.parentElement?.clientWidth || window.innerWidth;
      this.currentScale = Math.max(1.0, this.naturalWidth / containerWidth);
    }
    this.applyTransform();
  },

  applyTransform() {
    const img = document.getElementById("image-viewer-img");
    const label = document.getElementById("image-viewer-zoom-label");
    if (img) {
      img.style.transform = `translate(${this.translateX}px, ${this.translateY}px) scale(${this.currentScale})`;
    }
    if (label) {
      label.textContent = `${Math.round(this.currentScale * 100)}%`;
    }
  },

  downloadImage() {
    if (!this.currentUrl) return;
    const a = document.createElement("a");
    a.href = this.currentUrl;
    a.download = this.currentTitle || "image.png";
    a.target = "_blank";
    document.body.appendChild(a);
    a.click();
    a.remove();
  },

  openExternal() {
    if (this.currentUrl) {
      window.open(this.currentUrl, "_blank");
    }
  }
};

// --- Antigravity 2.31 Plugin Card & Jupyter Notebook Handlers ---
window.installPluginFromCard = async function(pluginId) {
  const cascadeId = (typeof activeCascadeId !== "undefined" && activeCascadeId) ||
                    (typeof currentCascadeId !== "undefined" && currentCascadeId) || "";
  try {
    const res = await fetch("/gateway/plugins/install", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ pluginId, cascadeId, autoResume: true })
    });
    if (res.ok) {
      alert("插件已成功安装！已自动为您继续对话。");
    } else {
      const err = await res.json().catch(() => ({}));
      alert("安装失败: " + (err.error || "未知错误"));
    }
  } catch (e) {
    alert("请求出错: " + e.message);
  }
};

window.renderJupyterNotebook = function(jsonStr) {
  try {
    const nb = typeof jsonStr === "string" ? JSON.parse(jsonStr) : jsonStr;
    if (!nb || !Array.isArray(nb.cells)) return null;
    let html = '<div class="ipynb-notebook">';
    nb.cells.forEach((cell) => {
      const cellType = cell.cell_type;
      const src = Array.isArray(cell.source) ? cell.source.join("") : (cell.source || "");
      if (cellType === "markdown") {
        html += `<div class="ipynb-cell ipynb-markdown-cell"><div class="ipynb-cell-prompt">Markdown</div><div class="markdown-content" style="padding:10px;">${renderMarkdown(src)}</div></div>`;
      } else if (cellType === "code") {
        const count = cell.execution_count != null ? cell.execution_count : " ";
        html += `<div class="ipynb-cell ipynb-code-cell"><div class="ipynb-cell-prompt">In [${count}]:</div><pre class="ipynb-code-area"><code>${escapeHtml(src)}</code></pre>`;
        if (Array.isArray(cell.outputs) && cell.outputs.length > 0) {
          html += '<div class="ipynb-output-area">';
          cell.outputs.forEach(out => {
            if (out.text) {
              const outText = Array.isArray(out.text) ? out.text.join("") : out.text;
              html += `<pre style="margin:0; font-family:ui-monospace, monospace; color:#a1a1aa;">${escapeHtml(outText)}</pre>`;
            } else if (out.data && out.data["image/png"]) {
              html += `<img src="data:image/png;base64,${out.data["image/png"]}" alt="Cell Output Image" />`;
            }
          });
          html += '</div>';
        }
        html += '</div>';
      }
    });
    html += '</div>';
    return html;
  } catch (e) {
    return null;
  }
};
