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
  // Blackboard bold & Mathcal
  "mathbb{R}": "ℝ", "mathbf{R}": "ℝ",
  "mathbb{N}": "ℕ", "mathbf{N}": "ℕ",
  "mathbb{Z}": "ℤ", "mathbf{Z}": "ℤ",
  "mathbb{Q}": "ℚ", "mathbf{Q}": "ℚ",
  "mathbb{C}": "ℂ", "mathbf{C}": "ℂ",
  "mathbb{E}": "𝔼", "mathbb{P}": "ℙ",
  "mathbb{H}": "ℍ", "mathbb{F}": "𝔽",
  "mathcal{L}": "ℒ", "mathcal{O}": "𝒪",
  "mathcal{N}": "𝒩", "mathcal{H}": "ℋ",
  "mathcal{F}": "ℱ", "mathcal{D}": "𝒟",

  // Long arrows
  "longleftrightarrow": "⟷", "Longleftrightarrow": "⟺",
  "longrightarrow": "⟶", "longleftarrow": "⟵",
  "Longrightarrow": "⟹", "Longleftarrow": "⟸",
  "longmapsto": "⟼",

  // Standard arrows & harpoons
  "rightleftharpoons": "⇌", "hookrightarrow": "↪", "hookleftarrow": "↩",
  "rightarrow": "→", "leftarrow": "←",
  "leftrightarrow": "↔", "Rightarrow": "⇒", "Leftarrow": "⇐", "Leftrightarrow": "⇔",
  "to": "→", "gets": "←", "implies": "⇒", "iff": "⇔",
  "uparrow": "↑", "downarrow": "↓", "updownarrow": "↕",
  "Uparrow": "⇑", "Downarrow": "⇓", "Updownarrow": "⇕",
  "nearrow": "↗", "searrow": "↘", "swarrow": "↙", "nwarrow": "↖",
  "mapsto": "↦",

  // Comparisons & Relations
  "leqslant": "≤", "geqslant": "≥", "leq": "≤", "geq": "≥",
  "le": "≤", "ge": "≥", "neq": "≠", "ne": "≠",
  "approx": "≈", "simeq": "≃", "cong": "≅", "equiv": "≡",
  "propto": "∝", "ll": "≪", "gg": "≫", "parallel": "∥", "perp": "⊥",
  "sim": "∼", "subset": "⊂", "supset": "⊃",
  "subseteq": "⊆", "supseteq": "⊇", "subsetneq": "⊊", "supsetneq": "⊋",
  "notin": "∉", "in": "∈", "cup": "∪", "cap": "∩", "setminus": "∖",
  "emptyset": "∅", "empty": "∅", "forall": "∀", "exists": "∃",

  // Operators & Calculus
  "times": "×", "div": "÷", "pm": "±", "mp": "∓",
  "cdot": "·", "cdots": "⋯", "ldots": "…", "vdots": "⋮", "ddots": "⋱",
  "bullet": "•", "circ": "∘", "star": "⋆", "ast": "∗",
  "oplus": "⊕", "ominus": "⊖", "otimes": "⊗", "odot": "⊙",
  "iiint": "∭", "iint": "∬", "oint": "∮", "int": "∫",
  "sum": "∑", "prod": "∏", "partial": "∂", "nabla": "∇", "infty": "∞", "sqrt": "√",
  "degree": "°",

  // Greek capital letters
  "Gamma": "Γ", "Delta": "Δ", "Theta": "Θ", "Lambda": "Λ", "Xi": "Ξ",
  "Pi": "Π", "Sigma": "Σ", "Upsilon": "Υ", "Phi": "Φ", "Psi": "Ψ", "Omega": "Ω",

  // Greek lowercase letters
  "alpha": "α", "beta": "β", "gamma": "γ", "delta": "δ",
  "varepsilon": "ε", "epsilon": "ϵ", "zeta": "ζ", "eta": "η",
  "vartheta": "ϑ", "theta": "θ", "iota": "ι", "kappa": "κ",
  "lambda": "λ", "mu": "μ", "nu": "ν", "xi": "ξ", "pi": "π",
  "varrho": "ϱ", "rho": "ρ", "varsigma": "ς", "sigma": "σ",
  "tau": "τ", "upsilon": "υ", "varphi": "φ", "phi": "ϕ",
  "chi": "χ", "psi": "ψ", "omega": "ω"
};

const wordBoundaryCommands = new Set(["to", "in", "le", "ge", "ne", "empty", "gets", "sim"]);

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
          if (latexSymbolLookup[paramCandidate]) {
            fullKey = paramCandidate;
            nextJ = closeIdx + 1;
          }
        }
      }

      if (fullKey && latexSymbolLookup[fullKey]) {
        // Word boundary check: if command is short, next char cannot be ASCII letter
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

const supMap = { "0": "⁰", "1": "¹", "2": "²", "3": "³", "4": "⁴", "5": "⁵", "6": "⁶", "7": "⁷", "8": "⁸", "9": "⁹", "+": "⁺", "-": "⁻", "=": "⁼", "(": "⁽", ")": "⁾", "n": "ⁿ", "i": "ⁱ", "j": "ʲ", "a": "ᵃ", "b": "ᵇ", "c": "ᶜ", "d": "ᵈ", "e": "ᵉ", "f": "ᶠ", "g": "ᵍ", "h": "ʰ", "k": "ᵏ", "l": "ˡ", "m": "ᵐ", "o": "ᵒ", "p": "ᵖ", "r": "ʳ", "s": "ˢ", "t": "ᵗ", "u": "ᵘ", "v": "ᵛ", "w": "ʷ", "x": "ˣ", "y": "ʸ", "z": "ᶻ", "T": "ᵀ" };
const subMap = { "0": "₀", "1": "₁", "2": "₂", "3": "₃", "4": "₄", "5": "₅", "6": "₆", "7": "₇", "8": "₈", "9": "₉", "+": "₊", "-": "₋", "=": "₌", "(": "₍", ")": "₎", "a": "ₐ", "e": "ₑ", "h": "ₕ", "i": "ᵢ", "j": "ⱼ", "k": "ₖ", "l": "ₗ", "m": "ₘ", "n": "ₙ", "o": "ₒ", "p": "ₚ", "r": "ᵣ", "s": "ₛ", "t": "ₜ", "u": "ᵤ", "v": "ᵥ", "x": "ₓ" };

function cleanMathExpr(str) {
  if (!str) return "";
  str = str.replace(/\\(?:text|mathrm|mathbf|mathit|operatorname)\{([^}]*)\}/g, "$1");
  str = str.replace(/\\frac\{([^}]*)\}\{([^}]*)\}/g, "$1 / $2");
  str = str.replace(/\\sqrt\{([^}]*)\}/g, "√($1)");
  str = str.replace(/\\left\(/g, "(").replace(/\\right\)/g, ")");
  str = str.replace(/\\left\[/g, "[").replace(/\\right\]/g, "]");
  str = str.replace(/\\left\\\{/g, "{").replace(/\\right\\\}/g, "}");
  str = str.replace(/\\\{/g, "{").replace(/\\\}/g, "}");
  str = str.replace(/\\%/g, "%").replace(/\\_/g, "_").replace(/\\&/g, "&");
  str = str.replace(/\\,/g, " ").replace(/\\;/g, " ").replace(/\\quad/g, " ").replace(/\\qquad/g, "  ");
  str = replaceSymbolsSinglePass(str);
  str = str.replace(/\^\{([0-9a-zA-Z\+\-\=\(\)]+)\}/g, (_, chars) => chars.split("").map(c => supMap[c] || c).join(""));
  str = str.replace(/\^([0-9a-zA-Z\+\-\*])/g, (_, c) => supMap[c] || c);
  str = str.replace(/_\{([0-9a-zA-Z\+\-\=\(\)]+)\}/g, (_, chars) => chars.split("").map(c => subMap[c] || c).join(""));
  str = str.replace(/_([0-9a-zA-Z])/g, (_, c) => subMap[c] || c);
  return str.trim();
}

function processMathSymbols(text) {
  if (!text) return "";
  // Fast-path: if text does not contain backslash or dollar sign, no LaTeX math can be present
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

  // Single-pass replacement for standalone LaTeX commands in prose
  text = replaceSymbolsSinglePass(text);

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
          <button class="agent-embed-action-btn" onclick="refreshAgentEmbedCard('${cardId}')" title="刷新交互组件" type="button">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M23 4v6h-6"></path>
              <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"></path>
            </svg>
          </button>
          <button class="agent-embed-action-btn" onclick="toggleFullscreenAgentEmbed('${cardId}')" title="全屏查看" type="button">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M8 3H5a2 2 0 0 0-2 2v3m18 0V5a2 2 0 0 0-2-2h-3m0 18h3a2 2 0 0 0 2-2v-3M3 16v3a2 2 0 0 0 2 2h3"></path>
            </svg>
          </button>
        </div>
      </div>
      <div class="agent-embed-viewport">
        <iframe class="agent-embed-frame" sandbox="allow-scripts allow-same-origin" src="about:blank"></iframe>
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

  // Normalize HTML whitespace entities (&nbsp;, &ensp;, &emsp;, &#160;) to unicode non-breaking spaces
  text = text.replace(/&(?:nbsp|#160|ensp|emsp);/gi, "\u00A0");

  // Normalize HTML <br> tags outside of inline code spans
  text = text.replace(/`[^`]+`|[ \t]*<(?:\/br|br\b[^>]*\/?)>[ \t]*\n?/gi, (match) => {
    if (match.startsWith("`")) return match;
    return "___HTML_BR___";
  });

  let html = escapeHtml(text);
  html = html.replace(/___HTML_BR___/g, "<br/>");

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
    const isMd = lower.endsWith(".md") || lower.endsWith(".markdown") || lower.includes("/brain/") || lower.includes("implementation_plan") || lower.includes("walkthrough");
    const isPlan = lower.includes("implementation_plan") || linkText.toLowerCase().includes("implementation_plan") || lower.includes("walkthrough") || linkText.toLowerCase().includes("walkthrough");
    const extraClass = isPlan ? " plan-btn-link" : (isMd ? " markdown-file-link" : "");
    const arrowSvg = isPlan ? '<svg class="plan-btn-arrow" width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"></path><polyline points="15 3 21 3 21 9"></polyline><line x1="10" y1="14" x2="21" y2="3"></line></svg>' : '';
    const href = escapeHtml(rawUrl);
    if (icon) {
      return `<a href="${href}" class="file-link${extraClass}" data-md-url="${href}" data-md-title="${escapeHtml(linkText)}"><img src="/icons/files/${icon}.svg" class="file-icon" alt="" /><span>${linkText}</span>${arrowSvg}</a>`;
    }
    return `<a href="${href}" class="text-link${extraClass}" data-md-url="${href}" data-md-title="${escapeHtml(linkText)}"><span>${linkText}</span>${arrowSvg}</a>`;
  });

  // Inline code (e.g. `foo`)
  html = html.replace(/`([^`]+)`/g, '<code class="inline-code">$1</code>');

  // Bold & Italic
  html = html.replace(/\*\*((?:[^*]|\*(?!\*))+?)\*\*/g, '<strong>$1</strong>');
  html = html.replace(/__((?:[^_]|_(?!_))+?)__/g, '<strong>$1</strong>');
  html = html.replace(/(?<!\*)\*([^*\n]+?)\*(?!\*)/g, '<em>$1</em>');
  html = html.replace(/(?<!_)_([^_\n]+?)_(?!_)/g, '<em>$1</em>');

  // Strikethrough
  html = html.replace(/~~((?:[^~]|~(?!~))+?)~~/g, '<del>$1</del>');

  // 5. Agent embed tag: <agent-embed src="..."></agent-embed>
  html = html.replace(/&lt;agent-embed\b[^&>]*?\bsrc=[&quot;']([^&quot;']+)&quot;[^&>]*&gt;(?:\s*&lt;\/agent-embed&gt;)?/gi, (_, src) => {
    return buildAgentEmbedCard(htmlUnescape(src));
  });

  return html;
}

function renderMarkdown(md) {
  if (!md) return "";
  md = processMathSymbols(md);

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
      if (fenceCount >= 3) {
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

      blocks.push(`
        <div class="code-block-card">
          <div class="code-block-header">
            <span class="code-block-lang">${displayLang}</span>
            <button class="code-copy-btn" onclick="copyCode(this)" type="button" aria-label="复制代码">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
                <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
              </svg>
              <span>复制</span>
            </button>
          </div>
          <pre class="code-block-pre"><code class="lang-${langClean}">${codeEscaped}</code></pre>
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

    // 4. Tables (| Header | Header |)
    if (trimmed.startsWith("|") && trimmed.endsWith("|") && trimmed.includes("|")) {
      const tableLines = [];
      while (i < lines.length) {
        const tLine = lines[i].trim();
        if (tLine.startsWith("|") && tLine.endsWith("|")) {
          tableLines.push(tLine);
          i++;
        } else {
          break;
        }
      }
      if (tableLines.length >= 2) {
        const parseTableRow = (rowStr) => {
          const placeholder = "\uE000";
          const sanitized = rowStr.replace(/\\\|/g, placeholder);
          const parts = sanitized.split("|");
          if (parts.length < 2) return [];
          return parts.slice(1, parts.length - 1).map(c => c.replace(/\uE000/g, "|").trim());
        };
        const headers = parseTableRow(tableLines[0]);
        let alignments = [];
        const rows = [];
        for (let rIdx = 1; rIdx < tableLines.length; rIdx++) {
          const r = parseTableRow(tableLines[rIdx]);
          // Skip separator row (| --- | :--- |)
          const isSep = r.length > 0 && r.every(cell => /^[\s\-:]+$/.test(cell));
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

    // 6. Lists (Unordered & Ordered)
    const isUnordered = trimmed.startsWith("- ") || trimmed.startsWith("* ") || trimmed.startsWith("• ");
    const isOrdered = /^\d+\.\s/.test(trimmed);
    if (isUnordered || isOrdered) {
      const listItems = [];
      const tag = isOrdered ? "ol" : "ul";
      while (i < lines.length) {
        const lLine = lines[i].trim();
        if (isOrdered && /^\d+\.\s/.test(lLine)) {
          listItems.push(lLine.replace(/^\d+\.\s+/, ""));
          i++;
        } else if (!isOrdered && (lLine.startsWith("- ") || lLine.startsWith("* ") || lLine.startsWith("• "))) {
          listItems.push(lLine.replace(/^[-*•]\s+/, ""));
          i++;
        } else {
          break;
        }
      }
      const itemsHtml = listItems.map(it => `<li>${renderInlineMarkdown(it)}</li>`).join("");
      blocks.push(`<${tag} class="ios-list">${itemsHtml}</${tag}>`);
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
      if (!nTrimmed ||
          nTrimmed.startsWith("MEDIA:") ||
          /^<agent-embed\b/i.test(nTrimmed) ||
          /^(?:https?:\/\/[^\s]+\.(?:png|jpe?g|webp|gif|svg|bmp)|(?:\/|[a-zA-Z]:\\|file:\/\/)[^\s<"']+\.(?:png|jpe?g|webp|gif|svg|bmp))$/i.test(nTrimmed) ||
          nTrimmed.startsWith("```") ||
          nTrimmed.startsWith("#") ||
          nTrimmed === "---" || nTrimmed === "***" || nTrimmed === "___" ||
          (nTrimmed.startsWith("|") && nTrimmed.endsWith("|")) ||
          nTrimmed.startsWith(">") ||
          nTrimmed.startsWith("- ") || nTrimmed.startsWith("* ") || nTrimmed.startsWith("• ") ||
          /^\d+\.\s/.test(nTrimmed)) {
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
      const result = await mermaid.render(uniqueId, code);
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
