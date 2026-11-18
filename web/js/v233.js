// Antigravity 2.33.0 Feature Pack for Web & PWA
// Covers: Nord Theme & Color-Scheme toggle, R/Rmd syntax highlighting,
// GitHub PR pills + detail <dialog>, Workspace Trust banner, /btw side-questions,
// Find-in-Page (<dialog popover="manual"> + TreeWalker + CSS.highlights),
// and 200 MB Audio/Video/.amr & PDF/Code modal preview.

(function () {
  const MAX_GENERAL_UPLOAD_BYTES = 50 * 1024 * 1024;
  const MAX_MEDIA_UPLOAD_BYTES = 200 * 1024 * 1024;

  const AUDIO_EXTS = new Set(["mp3", "m4a", "wav", "aac", "flac", "ogg", "oga", "opus", "aif", "aiff", "wma", "amr", "caf"]);
  const VIDEO_EXTS = new Set(["mp4", "mov", "mkv", "webm", "avi", "flv", "wmv", "m4v", "3gp"]);
  const R_LANGS = new Set(["r", "rscript", "rmd", "rmarkdown", "rprofile", "renviron", "namespace"]);

  const PR_URL_RE = /https?:\/\/([^/\s]+)\/([A-Za-z0-9_.-]+)\/([A-Za-z0-9_.-]+)\/pull\/(\d+)(?:[/?#][^\s)\]"'<>]*)?/gi;

  const prCache = new Map();
  const prInFlight = new Set();
  const dismissedTrust = new Set();

  let currentFindRoot = null;
  let currentFindMatches = [];
  let currentFindIndex = -1;
  let fallbackMarks = [];

  let activeBtwParentId = "";
  let activeBtwCascadeId = "";
  let activeBtwQuote = "";
  let activeBtwInsertText = "";
  let btwPollTimer = 0;

  function fileExt(nameOrUrl) {
    const clean = String(nameOrUrl || "").split("?")[0].split("#")[0].toLowerCase();
    const dot = clean.lastIndexOf(".");
    return dot >= 0 ? clean.slice(dot + 1) : "";
  }

  function isAudioOrVideoFile(file) {
    const t = String(file?.type || "").toLowerCase();
    if (t.startsWith("audio/") || t.startsWith("video/")) return true;
    const ext = fileExt(file?.name);
    return AUDIO_EXTS.has(ext) || VIDEO_EXTS.has(ext);
  }

  function maxUploadBytesForFile(file) {
    return isAudioOrVideoFile(file) ? MAX_MEDIA_UPLOAD_BYTES : MAX_GENERAL_UPLOAD_BYTES;
  }

  // --- 1. Theme & Color Scheme (system | light | dark + independent Light/Dark presets) ---
  const THEME_STORAGE_KEY = "antigravity_theme";
  const LIGHT_PRESET_KEY = "antigravity_light_preset";
  const DARK_PRESET_KEY = "antigravity_dark_preset";

  const LIGHT_PRESETS = [
    { id: "default", name: "Apple 默认", desc: "原生浅灰与极简白卡片", group: "default", isDefaultStyle: true,
      bg: "#F2F2F7", surface: "#FFFFFF", fg: "#000000", accent: "#007AFF", accentFg: "#FFFFFF",
      tok: { kw: "#ad3da4", str: "#c41a16", cmt: "#6c7986", num: "#1c00cf", type: "#0b4f79", fn: "#326d74", attr: "#6f42c1", meta: "#8250df", diffAdd: "#1a7f37", diffDel: "#cf222e" } },
    { id: "desktop-light", name: "Default Light", desc: "Antigravity 桌面默认浅色", group: "default",
      bg: "#FFFFFF", fg: "#0A0A0C", accent: "#2F75C3", accentFg: "#FFFFFF",
      tok: { kw: "#07a", str: "#5c8a00", cmt: "#708090", num: "#905", type: "#dd4a68", fn: "#dd4a68", attr: "#5c8a00", meta: "#9a6e3a", diffAdd: "#5c8a00", diffDel: "#905" } },
    { id: "legacy-light", name: "Legacy Light", desc: "经典浅灰高对比工作区", group: "default",
      bg: "#F9F9F9", fg: "#101010", accent: "#007ACC", accentFg: "#FFFFFF",
      tok: { kw: "#07a", str: "#5c8a00", cmt: "#708090", num: "#905", type: "#dd4a68", fn: "#dd4a68", attr: "#5c8a00", meta: "#9a6e3a", diffAdd: "#5c8a00", diffDel: "#905" } },
    { id: "catppuccin-light", name: "Catppuccin", desc: "Latte 柔和薰衣草粉紫", group: "desktop",
      bg: "#EAECF0", fg: "#4C4F69", accent: "#8839EF", accentFg: "#FFFFFF",
      tok: { kw: "#8839EF", str: "#40A02B", cmt: "#7C7F93", num: "#FE640B", type: "#DF8E1D", fn: "#1E66F5", attr: "#DF8E1D", meta: "#EA76CB", diffAdd: "#40A02B", diffDel: "#D20F39" } },
    { id: "everforest-light", name: "Everforest", desc: "护眼暖米纸张与苔藓绿", group: "desktop",
      bg: "#F8F0DC", fg: "#5C6A72", accent: "#4E7525", accentFg: "#F8F0DC", codeFg: "#2D7A58",
      tok: { kw: "#d13835", str: "#547300", cmt: "#708073", num: "#b84692", type: "#a67200", fn: "#247a58", attr: "#a67200", meta: "#c75b12", diffAdd: "#547300", diffDel: "#d13835" } },
    { id: "gruvbox-light", name: "Gruvbox Material", desc: "复古暖羊皮纸与琥珀橙", group: "desktop",
      bg: "#F9F5D7", fg: "#4F3829", accent: "#C35E0A", accentFg: "#FFFFFF",
      tok: { kw: "#c14a4a", str: "#6c782e", cmt: "#928374", num: "#945e80", type: "#b47109", fn: "#6c782e", attr: "#b47109", meta: "#4c7a5d", diffAdd: "#6c782e", diffDel: "#c14a4a" } },
    { id: "one-light", name: "One Light", desc: "经典 Atom 素净浅灰", group: "desktop",
      bg: "#F5F5F5", fg: "#383A42", accent: "#4078F2", accentFg: "#FFFFFF",
      tok: { kw: "#A626A4", str: "#449344", cmt: "#A0A1A7", num: "#986801", type: "#C18401", fn: "#4078F2", attr: "#986801", meta: "#0184BC", diffAdd: "#449344", diffDel: "#E45649" } },
    { id: "solarized-light", name: "Solarized Light", desc: "经典低眩光暖沙色", group: "desktop",
      bg: "#FAF4E5", fg: "#435155", accent: "#CB4B16", accentFg: "#FFFFFF",
      tok: { kw: "#859900", str: "#2AA198", cmt: "#93A1A1", num: "#D33682", type: "#B58900", fn: "#268BD2", attr: "#93A1A1", meta: "#CB4B16", diffAdd: "#859900", diffDel: "#DC322F" } }
  ];

  const DARK_PRESETS = [
    { id: "default", name: "Apple 默认", desc: "纯黑 OLED 与深灰分层", group: "default", isDefaultStyle: true,
      bg: "#000000", surface: "#1C1C1E", fg: "#FFFFFF", accent: "#5E5CE6", accentFg: "#FFFFFF",
      tok: { kw: "#ff7ab2", str: "#fc6a5d", cmt: "#7f8c98", num: "#d0bf69", type: "#6bdfff", fn: "#67b7a4", attr: "#a167e6", meta: "#fd8f3f", diffAdd: "#30d158", diffDel: "#ff453a" } },
    { id: "desktop-dark", name: "Default Dark", desc: "Antigravity 桌面默认深邃黑", group: "default",
      bg: "#070708", fg: "#FFFFFF", accent: "#2F71E3", accentFg: "#FFFFFF",
      tok: { kw: "#66d9ef", str: "#a6e22e", cmt: "#8292a2", num: "#ae81ff", type: "#e6db74", fn: "#e6db74", attr: "#a6e22e", meta: "#fd971f", diffAdd: "#a6e22e", diffDel: "#f92672" } },
    { id: "legacy-dark", name: "Legacy Dark", desc: "经典炭黑高可读性", group: "default",
      bg: "#101010", fg: "#CCCCCC", accent: "#007ACC", accentFg: "#FFFFFF",
      tok: { kw: "#66d9ef", str: "#a6e22e", cmt: "#8292a2", num: "#ae81ff", type: "#e6db74", fn: "#e6db74", attr: "#a6e22e", meta: "#fd971f", diffAdd: "#a6e22e", diffDel: "#f92672" } },
    { id: "tokyo-night", name: "Tokyo Night", desc: "东京夜色深靛蓝与霓虹蓝", group: "desktop",
      bg: "#1A1B26", fg: "#A9B1D6", accent: "#7AA2F7", accentFg: "#1A1B26",
      tok: { kw: "#BB9AF7", str: "#9ECE6A", cmt: "#565F89", num: "#FF9E64", type: "#2AC3DE", fn: "#7AA2F7", attr: "#7AA2F7", meta: "#89DDFF", diffAdd: "#9ECE6A", diffDel: "#F7768E" } },
    { id: "nord", name: "Nord", desc: "极地冰蓝冷调暗色", group: "desktop",
      bg: "#2E3440", surface: "#3B4252", surfaceVariant: "#434C5E", border: "#4C566A", fg: "#D8DEE9", accent: "#88C0D0", accentFg: "#2E3440", userBubbleBg: "#5E81AC", userBubbleText: "#ECEFF4", codeBlockBg: "#2E3440",
      tok: { kw: "#81A1C1", str: "#A3BE8C", cmt: "#616E88", num: "#B48EAD", type: "#8FBCBB", fn: "#88C0D0", attr: "#81A1C1", meta: "#EBCB8B", diffAdd: "#A3BE8C", diffDel: "#BF616A" } },
    { id: "catppuccin-dark", name: "Catppuccin", desc: "Macchiato 柔和粉紫暮色", group: "desktop",
      bg: "#24273A", fg: "#CAD3F5", accent: "#C6A0F6", accentFg: "#24273A",
      tok: { kw: "#C6A0F6", str: "#A6DA95", cmt: "#939AB7", num: "#F5A97F", type: "#EED49F", fn: "#8AADF4", attr: "#EED49F", meta: "#F5BDE6", diffAdd: "#A6DA95", diffDel: "#ED8796" } },
    { id: "dracula", name: "Dracula", desc: "德古拉经典暗紫夜色", group: "desktop",
      bg: "#282A36", fg: "#F8F8F2", accent: "#BD93F9", accentFg: "#282A36",
      tok: { kw: "#FF79C6", str: "#F1FA8C", cmt: "#6272A4", num: "#BD93F9", type: "#8BE9FD", fn: "#50FA7B", attr: "#50FA7B", meta: "#FF79C6", diffAdd: "#50FA7B", diffDel: "#FF5555" } },
    { id: "everforest-dark", name: "Everforest", desc: "松林暗绿与柔光暖字", group: "desktop",
      bg: "#2D353B", fg: "#D3C6AA", accent: "#A7C080", accentFg: "#232A2E", codeFg: "#A7C080",
      tok: { kw: "#e67e80", str: "#a7c080", cmt: "#9aa69d", num: "#d699b6", type: "#dbbc7f", fn: "#83c092", attr: "#dbbc7f", meta: "#e69875", diffAdd: "#a7c080", diffDel: "#e67e80" } },
    { id: "gruvbox-dark", name: "Gruvbox Material", desc: "复古炭黑与暖橘琥珀", group: "desktop",
      bg: "#141617", fg: "#D4BE98", accent: "#E78A4E", accentFg: "#141617", codeFg: "#A9B665",
      tok: { kw: "#ea6962", str: "#a9b665", cmt: "#7c6f64", num: "#d3869b", type: "#d8a657", fn: "#a9b665", attr: "#d8a657", meta: "#89b482", diffAdd: "#a9b665", diffDel: "#ea6962" } },
    { id: "monokai", name: "Monokai", desc: "高对比经典亮粉代码风", group: "desktop",
      bg: "#272822", fg: "#F8F8F2", accent: "#F92672", accentFg: "#FFFFFF",
      tok: { kw: "#F92672", str: "#E6DB74", cmt: "#75715E", num: "#AE81FF", type: "#A6E22E", fn: "#A6E22E", attr: "#A6E22E", meta: "#FD971F", diffAdd: "#A6E22E", diffDel: "#F92672" } },
    { id: "one-dark-pro", name: "One Dark Pro", desc: "Atom / VS Code 经典灰蓝", group: "desktop",
      bg: "#282C34", fg: "#ABB2BF", accent: "#61AFEF", accentFg: "#1E222A",
      tok: { kw: "#C678DD", str: "#98C379", cmt: "#5C6370", num: "#D19A66", type: "#E5C07B", fn: "#61AFEF", attr: "#D19A66", meta: "#56B6C2", diffAdd: "#98C379", diffDel: "#E06C75" } },
    { id: "solarized-dark", name: "Solarized Dark", desc: "深青绿护眼学术配色", group: "desktop",
      bg: "#002B36", fg: "#93A1A1", accent: "#268BD2", accentFg: "#FFFFFF",
      tok: { kw: "#859900", str: "#2AA198", cmt: "#586E75", num: "#D33682", type: "#B58900", fn: "#268BD2", attr: "#586E75", meta: "#CB4B16", diffAdd: "#859900", diffDel: "#DC322F" } },
    { id: "vesper", name: "Vesper", desc: "极简纯黑与暖杏桃点缀", group: "desktop",
      bg: "#101010", fg: "#FFFFFF", accent: "#FFC799", accentFg: "#101010",
      tok: { kw: "#FBADFF", str: "#99FFE4", cmt: "#7C7C7C", num: "#FFC799", type: "#FFC799", fn: "#FFC799", attr: "#FFC799", meta: "#A0A0A0", diffAdd: "#99FFE4", diffDel: "#F85149" } }
  ];

  const DYNAMIC_CSS_VARS = [
    "--ios-bg", "--ios-secondary-bg", "--ios-tertiary-bg", "--ios-quaternary-bg",
    "--ios-fill", "--ios-separator", "--ios-separator-subtle",
    "--ios-label", "--ios-secondary-label", "--ios-tertiary-label", "--ios-quaternary-label",
    "--ios-blue", "--ios-green", "--ios-indigo", "--ios-indigo-active", "--ios-orange", "--ios-red", "--ios-purple", "--ios-teal",
    "--orchestrator-slate", "--orchestrator-slate-rgb",
    "--code-fg", "--code-bg", "--code-border",
    "--tok-kw", "--tok-str", "--tok-cmt", "--tok-num", "--tok-type", "--tok-fn", "--tok-attr", "--tok-meta", "--tok-diff-add", "--tok-diff-del",
    "--user-bubble-bg", "--user-bubble-text", "--agent-bubble-bg", "--agent-bubble-text",
    "--navbar-bg", "--alert-bg", "--code-card-bg", "--code-card-border"
  ];

  function parseHexRgb(hex) {
    const clean = String(hex || "").replace(/^#/, "");
    const full = clean.length === 3 ? clean.split("").map((c) => c + c).join("") : clean;
    if (!/^[0-9a-fA-F]{6}$/.test(full)) return [128, 128, 128];
    return [parseInt(full.slice(0, 2), 16), parseInt(full.slice(2, 4), 16), parseInt(full.slice(4, 6), 16)];
  }

  function mixHex(hexA, hexB, weightA) {
    const [r1, g1, b1] = parseHexRgb(hexA);
    const [r2, g2, b2] = parseHexRgb(hexB);
    const r = Math.round(r1 * weightA + r2 * (1 - weightA));
    const g = Math.round(g1 * weightA + g2 * (1 - weightA));
    const b = Math.round(b1 * weightA + b2 * (1 - weightA));
    return `#${[r, g, b].map((x) => Math.max(0, Math.min(255, x)).toString(16).padStart(2, "0")).join("")}`;
  }

  function hexAlpha(hex, alpha) {
    const [r, g, b] = parseHexRgb(hex);
    return `rgba(${r}, ${g}, ${b}, ${alpha})`;
  }

  function migrateThemeStorage() {
    try {
      let mode = localStorage.getItem(THEME_STORAGE_KEY) || "system";
      if (mode === "nord") {
        mode = "dark";
        localStorage.setItem(THEME_STORAGE_KEY, "dark");
        if (!localStorage.getItem(DARK_PRESET_KEY)) {
          localStorage.setItem(DARK_PRESET_KEY, "nord");
        }
      }
      if (!["system", "light", "dark"].includes(mode)) {
        mode = "system";
        localStorage.setItem(THEME_STORAGE_KEY, "system");
      }
    } catch (_) {}
  }

  function getThemeState() {
    migrateThemeStorage();
    let mode = "system";
    let lightPreset = "default";
    let darkPreset = "default";
    try {
      mode = localStorage.getItem(THEME_STORAGE_KEY) || "system";
      lightPreset = localStorage.getItem(LIGHT_PRESET_KEY) || "default";
      darkPreset = localStorage.getItem(DARK_PRESET_KEY) || "default";
    } catch (_) {}
    if (!LIGHT_PRESETS.some((p) => p.id === lightPreset)) lightPreset = "default";
    if (!DARK_PRESETS.some((p) => p.id === darkPreset)) darkPreset = "default";
    return { mode, lightPreset, darkPreset };
  }

  function resolveEffectiveAppearance() {
    const { mode, lightPreset, darkPreset } = getThemeState();
    const systemDark = window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches;
    const isDark = mode === "dark" || (mode === "system" && systemDark);
    const list = isDark ? DARK_PRESETS : LIGHT_PRESETS;
    const presetId = isDark ? darkPreset : lightPreset;
    const preset = list.find((p) => p.id === presetId) || list[0];
    return { mode, isDark, lightPreset, darkPreset, preset };
  }

  function applyPresetVariables(preset, isDark) {
    const rootStyle = document.documentElement.style;
    if (preset.isDefaultStyle) {
      for (const k of DYNAMIC_CSS_VARS) rootStyle.removeProperty(k);
      document.documentElement.removeAttribute("data-theme-preset");
      return;
    }

    document.documentElement.setAttribute("data-theme-preset", preset.id);
    const bg = preset.bg;
    const fg = preset.fg;
    const accent = preset.accent;
    const accentFg = preset.accentFg || "#FFFFFF";
    const surface = preset.surface || (isDark ? mixHex("#FFFFFF", bg, 0.065) : mixHex("#FFFFFF", bg, 0.72));
    const surfaceVar = preset.surfaceVariant || (isDark ? mixHex("#FFFFFF", bg, 0.12) : mixHex(fg, bg, 0.08));
    const quatBg = isDark ? mixHex("#FFFFFF", bg, 0.17) : mixHex(fg, bg, 0.14);
    const borderHex = preset.border || mixHex(fg, bg, isDark ? 0.22 : 0.16);
    const secLabel = isDark ? mixHex(fg, bg, 0.72) : mixHex(fg, bg, 0.70);
    const tertLabel = mixHex(fg, bg, 0.48);
    const quatLabel = mixHex(fg, bg, 0.30);
    const codeBgHex = preset.codeBlockBg || (isDark ? mixHex("#000000", bg, 0.16) : mixHex(fg, bg, 0.055));
    const codeFgHex = preset.codeFg || preset.tok.meta || accent;
    const [orchR, orchG, orchB] = parseHexRgb(surfaceVar);

    const vars = {
      "--ios-bg": bg,
      "--ios-secondary-bg": surface,
      "--ios-tertiary-bg": surfaceVar,
      "--ios-quaternary-bg": quatBg,
      "--ios-fill": hexAlpha(fg, isDark ? 0.11 : 0.08),
      "--ios-separator": hexAlpha(borderHex, 0.78),
      "--ios-separator-subtle": hexAlpha(borderHex, 0.45),
      "--ios-label": fg,
      "--ios-secondary-label": secLabel,
      "--ios-tertiary-label": tertLabel,
      "--ios-quaternary-label": quatLabel,
      "--ios-blue": accent,
      "--ios-green": preset.tok.diffAdd,
      "--ios-indigo": accent,
      "--ios-indigo-active": mixHex("#000000", accent, 0.18),
      "--ios-orange": preset.tok.meta,
      "--ios-red": preset.tok.diffDel,
      "--ios-purple": preset.tok.num,
      "--ios-teal": preset.tok.type,
      "--orchestrator-slate": surfaceVar,
      "--orchestrator-slate-rgb": `${orchR}, ${orchG}, ${orchB}`,
      "--code-fg": codeFgHex,
      "--code-bg": hexAlpha(accent, isDark ? 0.14 : 0.09),
      "--code-border": hexAlpha(accent, 0.26),
      "--tok-kw": preset.tok.kw,
      "--tok-str": preset.tok.str,
      "--tok-cmt": preset.tok.cmt,
      "--tok-num": preset.tok.num,
      "--tok-type": preset.tok.type,
      "--tok-fn": preset.tok.fn,
      "--tok-attr": preset.tok.attr,
      "--tok-meta": preset.tok.meta,
      "--tok-diff-add": preset.tok.diffAdd,
      "--tok-diff-del": preset.tok.diffDel,
      "--user-bubble-bg": preset.userBubbleBg || accent,
      "--user-bubble-text": preset.userBubbleText || accentFg,
      "--agent-bubble-bg": surface,
      "--agent-bubble-text": fg,
      "--navbar-bg": hexAlpha(bg, 0.88),
      "--alert-bg": hexAlpha(surface, 0.95),
      "--code-card-bg": codeBgHex,
      "--code-card-border": hexAlpha(borderHex, 0.82)
    };
    for (const [k, v] of Object.entries(vars)) {
      rootStyle.setProperty(k, v);
    }
  }

  let activeSettingsSubpage = null; // null | "light-themes" | "dark-themes" | "compact"

  function renderSwatchDots(preset) {
    return `<span class="theme-dot" style="background:${preset.bg}"></span><span class="theme-dot" style="background:${preset.fg}"></span><span class="theme-dot" style="background:${preset.accent}"></span>`;
  }

  function updateThemeSettingsUI() {
    const { mode, lightPreset, darkPreset } = getThemeState();
    document.querySelectorAll("#settings-theme-mode-seg .settings-seg-btn").forEach((btn) => {
      const active = btn.getAttribute("data-theme-mode") === mode;
      btn.classList.toggle("active", active);
      btn.setAttribute("aria-pressed", active ? "true" : "false");
    });

    const lightBtn = document.getElementById("btn-open-light-themes");
    const darkBtn = document.getElementById("btn-open-dark-themes");
    if (lightBtn) lightBtn.classList.toggle("hidden", mode === "dark");
    if (darkBtn) darkBtn.classList.toggle("hidden", mode === "light");

    const lObj = LIGHT_PRESETS.find((p) => p.id === lightPreset) || LIGHT_PRESETS[0];
    const dObj = DARK_PRESETS.find((p) => p.id === darkPreset) || DARK_PRESETS[0];
    const lNameEl = document.getElementById("settings-light-preset-name");
    const dNameEl = document.getElementById("settings-dark-preset-name");
    const lSwatchEl = document.getElementById("settings-light-swatch");
    const dSwatchEl = document.getElementById("settings-dark-swatch");
    if (lNameEl) lNameEl.textContent = lObj.name;
    if (dNameEl) dNameEl.textContent = dObj.name;
    if (lSwatchEl) lSwatchEl.innerHTML = renderSwatchDots(lObj);
    if (dSwatchEl) dSwatchEl.innerHTML = renderSwatchDots(dObj);

    if (activeSettingsSubpage === "light-themes" || activeSettingsSubpage === "dark-themes") {
      renderThemePickerSubpage(activeSettingsSubpage === "dark-themes");
    }
  }

  function renderThemePickerSubpage(isDarkList) {
    const list = isDarkList ? DARK_PRESETS : LIGHT_PRESETS;
    const state = getThemeState();
    const selectedId = isDarkList ? state.darkPreset : state.lightPreset;
    const selectedPreset = list.find((p) => p.id === selectedId) || list[0];

    const pvCard = document.getElementById("settings-theme-preview");
    const pvName = document.getElementById("theme-preview-name");
    if (pvName) pvName.textContent = selectedPreset.name;
    if (pvCard) {
      const surface = selectedPreset.surface || (isDarkList ? mixHex("#FFFFFF", selectedPreset.bg, 0.065) : mixHex("#FFFFFF", selectedPreset.bg, 0.72));
      const codeBg = selectedPreset.codeBlockBg || (isDarkList ? mixHex("#000000", selectedPreset.bg, 0.16) : mixHex(selectedPreset.fg, selectedPreset.bg, 0.055));
      const border = selectedPreset.border || mixHex(selectedPreset.fg, selectedPreset.bg, isDarkList ? 0.22 : 0.16);
      pvCard.style.background = selectedPreset.bg;
      pvCard.style.borderColor = border;
      const uBubble = pvCard.querySelector(".theme-preview-bubble-user");
      const aBubble = pvCard.querySelector(".theme-preview-bubble-agent");
      const codePre = pvCard.querySelector(".theme-preview-code");
      if (uBubble) {
        uBubble.style.background = selectedPreset.userBubbleBg || selectedPreset.accent;
        uBubble.style.color = selectedPreset.userBubbleText || selectedPreset.accentFg || "#FFFFFF";
      }
      if (aBubble) {
        aBubble.style.background = surface;
        aBubble.style.color = selectedPreset.fg;
        aBubble.style.borderColor = border;
      }
      if (codePre) {
        codePre.style.background = codeBg;
        codePre.style.borderColor = border;
        codePre.style.color = selectedPreset.fg;
      }
      const setTok = (cls, col) => {
        const el = pvCard.querySelector(cls);
        if (el) el.style.color = col;
      };
      setTok(".pv-kw", selectedPreset.tok.kw);
      setTok(".pv-fn", selectedPreset.tok.fn);
      setTok(".pv-var", selectedPreset.fg);
      setTok(".pv-op", selectedPreset.tok.meta);
      setTok(".pv-str", selectedPreset.tok.str);
      setTok(".pv-cmt", selectedPreset.tok.cmt);
    }

    const defBox = document.getElementById("settings-theme-list-default");
    const presetBox = document.getElementById("settings-theme-list-presets");
    const buildRows = (items) => items.map((p) => {
      const checked = p.id === selectedId;
      return `<button type="button" class="ios-cell-button between theme-preset-row${checked ? " selected" : ""}" data-preset-id="${p.id}">
        <span class="theme-preset-main">
          <span class="theme-preset-pill" style="background:${p.bg};border-color:${p.border || hexAlpha(p.fg, 0.22)};">
            <span class="theme-preset-bar" style="background:${p.accent};"></span>
            <span class="theme-preset-fg" style="background:${p.fg};"></span>
          </span>
          <span class="theme-preset-text">
            <span class="theme-preset-title">${escapeHtmlLocal(p.name)}</span>
            <span class="theme-preset-desc">${escapeHtmlLocal(p.desc)}</span>
          </span>
        </span>
        <span class="theme-preset-check">${checked ? "✓" : ""}</span>
      </button>`;
    }).join("");

    if (defBox) defBox.innerHTML = buildRows(list.filter((p) => p.group === "default"));
    if (presetBox) presetBox.innerHTML = buildRows(list.filter((p) => p.group !== "default"));

    const bindContainer = (box) => {
      box?.querySelectorAll(".theme-preset-row").forEach((row) => {
        row.addEventListener("click", () => {
          const id = row.getAttribute("data-preset-id");
          if (!id) return;
          if (isDarkList) {
            setDarkPreset(id);
          } else {
            setLightPreset(id);
          }
        });
      });
    };
    bindContainer(defBox);
    bindContainer(presetBox);
  }

  function openSettingsSubpage(kind) {
    activeSettingsSubpage = kind || null;
    const headerMain = document.getElementById("settings-header-main");
    const headerSub = document.getElementById("settings-header-sub");
    const mainBody = document.getElementById("settings-main-body");
    const subBody = document.getElementById("settings-sub-body");
    const subThemes = document.getElementById("settings-sub-themes");
    const subCompact = document.getElementById("settings-sub-compact");
    const subTitle = document.getElementById("settings-sub-title");

    if (!activeSettingsSubpage) {
      headerMain?.classList.remove("hidden");
      headerSub?.classList.add("hidden");
      mainBody?.classList.remove("hidden");
      subBody?.classList.add("hidden");
      subThemes?.classList.add("hidden");
      subCompact?.classList.add("hidden");
      return;
    }

    headerMain?.classList.add("hidden");
    headerSub?.classList.remove("hidden");
    mainBody?.classList.add("hidden");
    subBody?.classList.remove("hidden");
    if (subBody) subBody.scrollTop = 0;

    if (activeSettingsSubpage === "compact") {
      if (subTitle) subTitle.textContent = "精简模式";
      subThemes?.classList.add("hidden");
      subCompact?.classList.remove("hidden");
    } else {
      const isDarkList = activeSettingsSubpage === "dark-themes";
      if (subTitle) subTitle.textContent = isDarkList ? "深色配色" : "浅色配色";
      subCompact?.classList.add("hidden");
      subThemes?.classList.remove("hidden");
      renderThemePickerSubpage(isDarkList);
    }
  }

  function applyTheme(mode) {
    if (mode !== undefined) {
      const normalized = mode === "nord" ? "dark" : (["system", "light", "dark"].includes(mode) ? mode : "system");
      try {
        localStorage.setItem(THEME_STORAGE_KEY, normalized);
        if (mode === "nord") localStorage.setItem(DARK_PRESET_KEY, "nord");
      } catch (_) {}
    }

    const eff = resolveEffectiveAppearance();
    const root = document.documentElement;
    const csMeta = document.querySelector('meta[name="color-scheme"]');
    const tcMeta = document.getElementById("meta-theme-color");

    if (eff.mode === "system") {
      root.removeAttribute("data-theme");
      if (csMeta) csMeta.setAttribute("content", "light dark");
    } else {
      root.setAttribute("data-theme", eff.mode);
      if (csMeta) csMeta.setAttribute("content", eff.mode);
    }

    applyPresetVariables(eff.preset, eff.isDark);
    if (tcMeta) {
      tcMeta.setAttribute("content", eff.preset.isDefaultStyle ? (eff.isDark ? "#0f172a" : "#f2f2f7") : eff.preset.bg);
    }
    updateThemeSettingsUI();
  }

  function setLightPreset(id) {
    if (!LIGHT_PRESETS.some((p) => p.id === id)) return;
    try { localStorage.setItem(LIGHT_PRESET_KEY, id); } catch (_) {}
    applyTheme();
  }

  function setDarkPreset(id) {
    if (!DARK_PRESETS.some((p) => p.id === id)) return;
    try { localStorage.setItem(DARK_PRESET_KEY, id); } catch (_) {}
    applyTheme();
  }

  function initThemeSettings() {
    applyTheme();

    document.querySelectorAll("#settings-theme-mode-seg .settings-seg-btn").forEach((btn) => {
      btn.addEventListener("click", () => {
        const mode = btn.getAttribute("data-theme-mode");
        if (mode) applyTheme(mode);
      });
    });

    document.getElementById("btn-open-light-themes")?.addEventListener("click", () => openSettingsSubpage("light-themes"));
    document.getElementById("btn-open-dark-themes")?.addEventListener("click", () => openSettingsSubpage("dark-themes"));
    document.getElementById("btn-open-compact-settings")?.addEventListener("click", () => openSettingsSubpage("compact"));
    document.getElementById("btn-settings-sub-back")?.addEventListener("click", () => openSettingsSubpage(null));
    document.getElementById("btn-sheet-settings-close-sub")?.addEventListener("click", () => {
      openSettingsSubpage(null);
      if (typeof closeSettingsSheet === "function") closeSettingsSheet();
    });

    const mql = window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)");
    if (mql) {
      const onMediaChange = () => {
        if (getThemeState().mode === "system") applyTheme();
      };
      if (typeof mql.addEventListener === "function") mql.addEventListener("change", onMediaChange);
      else if (typeof mql.addListener === "function") mql.addListener(onMediaChange);
    }

    window.addEventListener("storage", (e) => {
      if (e.key === THEME_STORAGE_KEY || e.key === LIGHT_PRESET_KEY || e.key === DARK_PRESET_KEY) {
        applyTheme();
      }
    });
  }

  // --- 2. R & R Markdown Syntax Highlighting ---
  const R_KEYWORDS = new Set([
    "if", "else", "repeat", "while", "function", "for", "in", "next", "break",
    "TRUE", "FALSE", "NULL", "Inf", "NaN", "NA", "NA_integer_", "NA_real_",
    "NA_complex_", "NA_character_", "library", "require", "source", "return",
    "switch", "tryCatch", "stop", "warning", "message", "local", "with"
  ]);

  function escapeHtmlLocal(s) {
    return String(s || "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function highlightRCode(rawCode) {
    const tokenRe = /(#[^\n]*)|("(?:\\[\s\S]|[^"\\])*"|'(?:\\[\s\S]|[^'\\])*'|`[^`\n]+`)|(<<-|->|->>|<-|%>%|\|>|%[A-Za-z0-9_.]+%|:::?|\b\d+(?:\.\d+)?(?:[eE][+-]?\d+)?[Li]?\b|\b(?:0x[0-9a-fA-F]+)\b)|(\b[A-Za-z_.][A-Za-z0-9_.]*)(?=\s*\()|(\b[A-Za-z_.][A-Za-z0-9_.]*\b)/g;
    let out = "";
    let last = 0;
    let m;
    while ((m = tokenRe.exec(rawCode)) !== null) {
      if (m.index > last) out += escapeHtmlLocal(rawCode.slice(last, m.index));
      const [full, cmt, str, opOrNum, fnName, ident] = m;
      if (cmt) {
        out += `<span style="color:var(--tok-cmt);">${escapeHtmlLocal(cmt)}</span>`;
      } else if (str) {
        out += `<span style="color:var(--tok-str);">${escapeHtmlLocal(str)}</span>`;
      } else if (opOrNum) {
        const isOp = opOrNum.startsWith("<") || opOrNum.startsWith("-") || opOrNum.startsWith("|") || opOrNum.startsWith("%") || opOrNum.startsWith(":");
        const color = isOp ? "var(--tok-kw)" : "var(--tok-num)";
        out += `<span style="color:${color};">${escapeHtmlLocal(opOrNum)}</span>`;
      } else if (fnName) {
        const color = R_KEYWORDS.has(fnName) ? "var(--tok-kw)" : "var(--tok-fn)";
        out += `<span style="color:${color};">${escapeHtmlLocal(fnName)}</span>`;
      } else if (ident) {
        if (R_KEYWORDS.has(ident)) {
          out += `<span style="color:var(--tok-kw);font-weight:600;">${escapeHtmlLocal(ident)}</span>`;
        } else {
          out += escapeHtmlLocal(ident);
        }
      } else {
        out += escapeHtmlLocal(full);
      }
      last = tokenRe.lastIndex;
    }
    if (last < rawCode.length) out += escapeHtmlLocal(rawCode.slice(last));
    return out;
  }

  function highlightRmdCode(rawCode) {
    const lines = String(rawCode || "").split("\n");
    let inChunk = false;
    let inYaml = false;
    const out = [];
    for (let i = 0; i < lines.length; i++) {
      const line = lines[i];
      const trimmed = line.trim();
      if (i === 0 && trimmed === "---") {
        inYaml = true;
        out.push(`<span style="color:var(--tok-meta);">${escapeHtmlLocal(line)}</span>`);
        continue;
      }
      if (inYaml) {
        if (trimmed === "---" || trimmed === "...") {
          inYaml = false;
          out.push(`<span style="color:var(--tok-meta);">${escapeHtmlLocal(line)}</span>`);
        } else {
          out.push(`<span style="color:var(--tok-attr);">${escapeHtmlLocal(line)}</span>`);
        }
        continue;
      }
      if (/^```\{[rR]\b[^}]*\}\s*$/.test(trimmed)) {
        inChunk = true;
        out.push(`<span style="color:var(--tok-meta);font-weight:600;">${escapeHtmlLocal(line)}</span>`);
        continue;
      }
      if (inChunk && /^```\s*$/.test(trimmed)) {
        inChunk = false;
        out.push(`<span style="color:var(--tok-meta);font-weight:600;">${escapeHtmlLocal(line)}</span>`);
        continue;
      }
      if (inChunk) {
        out.push(highlightRCode(line));
      } else if (/^#{1,6}\s/.test(trimmed)) {
        out.push(`<span style="color:var(--tok-kw);font-weight:700;">${escapeHtmlLocal(line)}</span>`);
      } else {
        out.push(escapeHtmlLocal(line));
      }
    }
    return out.join("\n");
  }

  function installRHighlighter() {
    if (typeof window.highlightCodeHtml !== "function" || window.highlightCodeHtml.__v233Wrapped) return;
    const orig = window.highlightCodeHtml;
    const wrapped = function (rawCode, langClean) {
      const lang = String(langClean || "").trim().toLowerCase();
      if (lang === "rmd" || lang === "rmarkdown") {
        return highlightRmdCode(rawCode);
      }
      if (R_LANGS.has(lang)) {
        return highlightRCode(rawCode);
      }
      return orig(rawCode, langClean);
    };
    wrapped.__v233Wrapped = true;
    window.highlightCodeHtml = wrapped;
    if (typeof highlightCodeHtml === "function") {
      try {
        highlightCodeHtml = wrapped;
      } catch (_) {}
    }
  }

  // --- 3. GitHub Pull Request Pills & Detail <dialog> ---
  function normalizePrUrl(raw) {
    PR_URL_RE.lastIndex = 0;
    const m = PR_URL_RE.exec(String(raw || ""));
    if (!m) return "";
    return `https://${m[1].toLowerCase()}/${m[2]}/${m[3]}/pull/${m[4]}`;
  }

  function prStateTone(state) {
    const s = String(state || "OPEN").toUpperCase();
    if (s === "MERGED") return "merged";
    if (s === "DRAFT") return "draft";
    if (s === "CLOSED") return "closed";
    return "open";
  }

  function renderPrPillsForBubble(bubble, urls) {
    const resolved = urls.map((u) => prCache.get(u)).filter(Boolean);
    if (!resolved.length) return;
    let row = bubble.querySelector(".v233-pr-pills-row");
    const sig = resolved.map((p) => `${p.url}:${p.state}:${p.title}`).join("|");
    if (row && row.getAttribute("data-pr-sig") === sig) return;
    if (!row) {
      row = document.createElement("div");
      row.className = "v233-pr-pills-row";
      bubble.appendChild(row);
    }
    row.setAttribute("data-pr-sig", sig);
    row.innerHTML = resolved
      .map((pr) => {
        const st = String(pr.state || "OPEN").toUpperCase();
        const tone = prStateTone(st);
        const key = encodeURIComponent(pr.url || "");
        return `<button type="button" class="v233-pr-pill" data-pr-url="${key}" title="${escapeHtmlLocal(pr.title || "")}">
          <span class="git-pr-badge ${tone}">${escapeHtmlLocal(st)}</span>
          <span class="git-pr-num">#${Number(pr.number) || ""}</span>
          <span class="v233-pr-pill-title">${escapeHtmlLocal(pr.title || `${pr.owner}/${pr.repo}`)}</span>
        </button>`;
      })
      .join("");
  }

  async function hydrateChatPRPills(containerEl) {
    const root = containerEl || document.getElementById("messages-stream");
    if (!root) return;
    const bubbles = Array.from(root.querySelectorAll(".bubble"));
    const urlsToFetch = [];

    for (const bubble of bubbles) {
      const links = Array.from(bubble.querySelectorAll("a[href]"));
      const prUrls = [];
      for (const a of links) {
        const norm = normalizePrUrl(a.getAttribute("href"));
        if (norm && !prUrls.includes(norm)) {
          prUrls.push(norm);
        }
      }
      if (!prUrls.length) continue;
      renderPrPillsForBubble(bubble, prUrls);
      for (const u of prUrls) {
        if (!prCache.has(u) && !prInFlight.has(u)) {
          prInFlight.add(u);
          urlsToFetch.push(u);
        }
      }
    }

    if (!urlsToFetch.length) return;
    try {
      const res = await fetch("/gateway/github/pull-requests", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ urls: urlsToFetch.slice(0, 12) }),
      });
      if (!res.ok) return;
      const data = await res.json();
      const list = Array.isArray(data.pullRequests) ? data.pullRequests : [];
      for (const pr of list) {
        const norm = normalizePrUrl(pr.url);
        if (norm) prCache.set(norm, { ...pr, url: norm });
      }
      // Re-pass to mount pills now that cache is warm
      for (const bubble of Array.from((document.getElementById("messages-stream") || root).querySelectorAll(".bubble"))) {
        const prUrls = [];
        for (const a of bubble.querySelectorAll("a[href]")) {
          const norm = normalizePrUrl(a.getAttribute("href"));
          if (norm && !prUrls.includes(norm)) prUrls.push(norm);
        }
        if (prUrls.length) renderPrPillsForBubble(bubble, prUrls);
      }
    } catch (_) {
    } finally {
      urlsToFetch.forEach((u) => prInFlight.delete(u));
    }
  }

  function openPRDetailModal(pr) {
    const dlg = document.getElementById("dialog-github-pr");
    if (!dlg || !pr) return;
    const stateEl = document.getElementById("pr-dialog-state");
    const titleEl = document.getElementById("pr-dialog-title");
    const bodyEl = document.getElementById("pr-dialog-body");
    const extEl = document.getElementById("pr-dialog-open-ext");
    const askBtn = document.getElementById("btn-pr-dialog-ask");

    const st = String(pr.state || "OPEN").toUpperCase();
    if (stateEl) {
      stateEl.className = `git-pr-badge ${prStateTone(st)}`;
      stateEl.textContent = st;
    }
    if (titleEl) {
      titleEl.textContent = `#${Number(pr.number) || ""} ${pr.title || ""}`.trim();
    }
    if (extEl) {
      extEl.href = pr.url || "#";
    }
    if (bodyEl) {
      const repoLabel = pr.owner && pr.repo ? `${pr.owner}/${pr.repo}` : "";
      const refLabel = pr.headRefName && pr.baseRefName ? `${pr.headRefName} → ${pr.baseRefName}` : "";
      const author = pr.authorLogin ? `@${pr.authorLogin}` : "";
      const bodyText = String(pr.body || "").trim();
      bodyEl.innerHTML = `
        <div class="v233-pr-meta-row">
          ${repoLabel ? `<span>📦 ${escapeHtmlLocal(repoLabel)}</span>` : ""}
          ${author ? `<span>👤 ${escapeHtmlLocal(author)}</span>` : ""}
          ${refLabel ? `<span>🌿 ${escapeHtmlLocal(refLabel)}</span>` : ""}
        </div>
        ${bodyText ? `<div class="v233-pr-body-pre">${escapeHtmlLocal(bodyText)}</div>` : `<div style="color:var(--ios-secondary-label);">无 Pull Request 描述正文</div>`}
      `;
    }
    if (askBtn) {
      askBtn.onclick = () => {
        dlg.close();
        const quote = `PR #${pr.number || ""} (${pr.state || "OPEN"}): ${pr.title || ""}\n${pr.url || ""}`;
        openBtwModal({ parentCascadeId: activeCascadeId || "", quote });
      };
    }
    if (typeof dlg.showModal === "function") dlg.showModal();
  }

  // --- 4. Workspace Trust Gate ---
  let currentTrustPayload = null;

  async function checkWorkspaceTrust(cascadeId, workspaceUri) {
    const banner = document.getElementById("workspace-trust-banner");
    if (!banner) return;

    let ws = String(workspaceUri || "").trim();
    if (!ws && cascadeId && typeof currentTrajectories === "object") {
      ws = currentTrajectories[cascadeId]?.workspaceUris?.[0] || "";
    }
    if (!ws && typeof activeDraftSession === "object" && activeDraftSession && !activeDraftSession.isPure) {
      ws = activeDraftSession.uri || activeDraftSession.path || "";
    }
    if (!ws || ws === "outside-of-project" || dismissedTrust.has(ws)) {
      banner.classList.add("hidden");
      currentTrustPayload = null;
      return;
    }

    try {
      const qs = new URLSearchParams();
      if (cascadeId) qs.set("cascadeId", cascadeId);
      if (ws) qs.set("workspaceUri", ws);
      const res = await fetch(`/gateway/projects/trust?${qs.toString()}`);
      if (!res.ok) return;
      const st = await res.json();
      if (st && st.ok && (st.isProjectTrusted === false || st.hasBlockedItems === true)) {
        currentTrustPayload = { cascadeId, workspaceUri: ws, projectId: st.projectId || "" };
        const desc = document.getElementById("wtb-workspace-path");
        if (desc) desc.textContent = ws.replace(/^file:\/\//, "");
        banner.classList.remove("hidden");
      } else {
        currentTrustPayload = null;
        banner.classList.add("hidden");
      }
    } catch (_) {
      banner.classList.add("hidden");
    }
  }

  function initTrustBanner() {
    document.getElementById("btn-dismiss-trust")?.addEventListener("click", () => {
      if (currentTrustPayload?.workspaceUri) dismissedTrust.add(currentTrustPayload.workspaceUri);
      document.getElementById("workspace-trust-banner")?.classList.add("hidden");
    });

    document.getElementById("btn-trust-workspace")?.addEventListener("click", async () => {
      if (!currentTrustPayload) return;
      const btn = document.getElementById("btn-trust-workspace");
      if (btn) {
        btn.disabled = true;
        btn.textContent = "授权中…";
      }
      try {
        const res = await fetch("/gateway/projects/trust", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ ...currentTrustPayload, trust: true }),
        });
        const d = await res.json().catch(() => ({}));
        if (!res.ok || d.ok === false) throw new Error(d.error || "设置失败");
        document.getElementById("workspace-trust-banner")?.classList.add("hidden");
        currentTrustPayload = null;
        window.showToast?.("已信任当前工作区");
      } catch (err) {
        window.showToast?.("信任设置失败: " + (err.message || "未知错误"), 2800);
      } finally {
        if (btn) {
          btn.disabled = false;
          btn.textContent = "信任工作区";
        }
      }
    });
  }

  // --- 5. /btw Side-Question Loop ---
  function stopBtwPoll() {
    if (btwPollTimer) {
      clearInterval(btwPollTimer);
      btwPollTimer = 0;
    }
  }

  function renderBtwQuote() {
    const card = document.getElementById("btw-quote-card");
    const txt = document.getElementById("btw-quote-text");
    if (!card || !txt) return;
    if (activeBtwQuote) {
      txt.textContent = activeBtwQuote;
      card.classList.remove("hidden");
    } else {
      txt.textContent = "";
      card.classList.add("hidden");
    }
  }

  function renderBtwStatusView(st) {
    const statusArea = document.getElementById("btw-status-area");
    const insertWrap = document.getElementById("btw-insert-actions");
    if (!statusArea) return;
    if (!st) {
      statusArea.innerHTML = `<div style="color:var(--ios-secondary-label);">在下方输入临时问题，独立旁路求解；解答完成后可一键插入主对话或作为独立会话继续。</div>`;
      insertWrap?.classList.add("hidden");
      return;
    }
    const thinkingHtml = st.thinking
      ? `<details class="thought-box" style="margin-bottom:10px;"><summary>🧠 推理过程 (${st.thinking.length} 字)</summary><div class="thought-content">${escapeHtmlLocal(st.thinking)}</div></details>`
      : "";
    const answerHtml = st.answer && typeof window.renderMarkdown === "function"
      ? `<div class="markdown-body">${window.renderMarkdown(st.answer)}</div>`
      : st.isRunning
      ? `<div style="display:flex;align-items:center;gap:8px;color:var(--ios-secondary-label);"><div class="ios-spinner small"></div><span>正在思考旁问…</span></div>`
      : st.errorMessage
      ? `<div style="color:var(--ios-red);">${escapeHtmlLocal(st.errorMessage)}</div>`
      : "";
    statusArea.innerHTML = `
      ${st.question ? `<div style="font-weight:600;margin-bottom:8px;">Q: ${escapeHtmlLocal(st.question)}</div>` : ""}
      ${thinkingHtml}
      ${answerHtml}
    `;
    if (st.answer) {
      activeBtwInsertText = st.insertText || `> ${st.question || ""}\n\n${st.answer}`;
      insertWrap?.classList.remove("hidden");
    } else if (!st.isRunning && activeBtwCascadeId) {
      insertWrap?.classList.remove("hidden");
    } else {
      insertWrap?.classList.add("hidden");
    }
  }

  async function pollBtwStatus(cascadeId) {
    if (!cascadeId) return;
    try {
      const res = await fetch(`/gateway/btw/status?cascadeId=${encodeURIComponent(cascadeId)}`);
      if (!res.ok) return;
      const st = await res.json();
      if (cascadeId !== activeBtwCascadeId) return;
      renderBtwStatusView(st);
      if (!st.isRunning) {
        stopBtwPoll();
      }
    } catch (_) {}
  }

  async function refreshBtwHistory(parentId) {
    const bar = document.getElementById("btw-history-bar");
    if (!bar || !parentId) {
      bar?.classList.add("hidden");
      return;
    }
    try {
      const res = await fetch(`/gateway/btw/list?cascadeId=${encodeURIComponent(parentId)}`);
      if (!res.ok) return;
      const data = await res.json();
      const items = Array.isArray(data.items) ? data.items : [];
      if (!items.length) {
        bar.classList.add("hidden");
        bar.innerHTML = "";
        return;
      }
      bar.classList.remove("hidden");
      bar.innerHTML = items
        .map((it) => {
          const cid = escapeHtmlLocal(it.cascadeId || "");
          const title = escapeHtmlLocal(it.title || it.question || cid.slice(0, 8));
          const active = it.cascadeId === activeBtwCascadeId ? " active" : "";
          return `<button type="button" class="v233-btw-hist-chip${active}" data-btw-id="${cid}">${title}</button>`;
        })
        .join("");
      bar.querySelectorAll("[data-btw-id]").forEach((btn) => {
        btn.addEventListener("click", () => {
          const cid = btn.getAttribute("data-btw-id");
          if (!cid) return;
          activeBtwCascadeId = cid;
          refreshBtwHistory(parentId);
          stopBtwPoll();
          pollBtwStatus(cid);
          btwPollTimer = setInterval(() => pollBtwStatus(cid), 1200);
        });
      });
    } catch (_) {}
  }

  async function startBtwQuestion(question) {
    const q = String(question || "").trim();
    if (!q || !activeBtwParentId) return;
    const sendBtn = document.getElementById("btn-btw-send");
    const input = document.getElementById("btw-question-input");
    if (sendBtn) sendBtn.disabled = true;
    try {
      const res = await fetch("/gateway/btw/start", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          parentCascadeId: activeBtwParentId,
          question: q,
          quote: activeBtwQuote || "",
          model: typeof activeModel !== "undefined" ? activeModel : "",
        }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok || !data.cascadeId) throw new Error(data.error || "发起 /btw 失败");
      activeBtwCascadeId = data.cascadeId;
      if (input) input.value = "";
      renderBtwStatusView({ question: q, isRunning: true });
      refreshBtwHistory(activeBtwParentId);
      stopBtwPoll();
      pollBtwStatus(data.cascadeId);
      btwPollTimer = setInterval(() => pollBtwStatus(data.cascadeId), 1200);
    } catch (err) {
      window.showToast?.(err.message || "发起 /btw 失败", 2800);
    } finally {
      if (sendBtn) sendBtn.disabled = false;
    }
  }

  function openBtwModal({ parentCascadeId, quote = "", initialQuestion = "" } = {}) {
    const dlg = document.getElementById("dialog-btw");
    if (!dlg) return;
    const parentId = parentCascadeId || (typeof activeCascadeId !== "undefined" ? activeCascadeId : "");
    if (!parentId) {
      window.showToast?.("请先进入一个会话后再使用 /btw");
      return;
    }
    activeBtwParentId = parentId;
    activeBtwQuote = String(quote || "").trim();
    activeBtwCascadeId = "";
    activeBtwInsertText = "";
    stopBtwPoll();

    renderBtwQuote();
    renderBtwStatusView(null);
    refreshBtwHistory(parentId);

    const input = document.getElementById("btw-question-input");
    if (input) input.value = initialQuestion || "";
    if (typeof dlg.showModal === "function" && !dlg.open) dlg.showModal();

    if (initialQuestion && initialQuestion.trim()) {
      startBtwQuestion(initialQuestion.trim());
    } else {
      setTimeout(() => input?.focus(), 60);
    }
  }

  function initBtwDialog() {
    const dlg = document.getElementById("dialog-btw");
    document.getElementById("btn-btw-dialog-close")?.addEventListener("click", () => {
      stopBtwPoll();
      dlg?.close();
    });
    dlg?.addEventListener("close", stopBtwPoll);
    document.getElementById("btn-btw-clear-quote")?.addEventListener("click", () => {
      activeBtwQuote = "";
      renderBtwQuote();
    });
    document.getElementById("btn-btw-send")?.addEventListener("click", () => {
      const q = document.getElementById("btw-question-input")?.value || "";
      startBtwQuestion(q);
    });
    document.getElementById("btw-question-input")?.addEventListener("keydown", (e) => {
      if (e.isComposing || e.keyCode === 229) return;
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        startBtwQuestion(e.target.value);
      }
    });
    document.getElementById("btn-btw-insert-main")?.addEventListener("click", () => {
      const chatInput = document.getElementById("chat-input");
      if (chatInput && activeBtwInsertText) {
        const prefix = chatInput.value.trim() ? `${chatInput.value.trim()}\n\n` : "";
        chatInput.value = prefix + activeBtwInsertText;
        chatInput.style.height = "auto";
        chatInput.style.height = Math.min(chatInput.scrollHeight, 120) + "px";
        if (typeof updateChatControls === "function") {
          updateChatControls(typeof isCascadeRunning === "function" ? isCascadeRunning(activeCascadeId) : false, null, false);
        }
      }
      stopBtwPoll();
      dlg?.close();
      setTimeout(() => chatInput?.focus(), 60);
    });
    document.getElementById("btn-btw-open-session")?.addEventListener("click", () => {
      if (!activeBtwCascadeId) return;
      stopBtwPoll();
      dlg?.close();
      if (typeof navigateTo === "function") navigateTo("#c=" + activeBtwCascadeId);
    });
  }

  // --- 6. In-App Find in Page (TreeWalker + CSS.highlights + <dialog popover="manual">) ---
  function clearFindHighlights() {
    if ("highlights" in CSS) {
      // MANDATORY: Clear previous highlights before registering new ones.
      CSS.highlights.clear();
    }
    if (fallbackMarks.length > 0) {
      for (const mark of fallbackMarks) {
        const parent = mark.parentNode;
        if (parent) {
          parent.replaceChild(document.createTextNode(mark.textContent || ""), mark);
          parent.normalize();
        }
      }
      fallbackMarks = [];
    }
    currentFindMatches = [];
    currentFindIndex = -1;
  }

  function collectTextNodes(rootEl) {
    // MANDATORY: Use TreeWalker to collect text nodes — do not manipulate innerHTML.
    const walker = document.createTreeWalker(rootEl, NodeFilter.SHOW_TEXT);
    const nodes = [];
    let node;
    while ((node = walker.nextNode())) {
      if (node.nodeValue && node.nodeValue.length > 0) nodes.push(node);
    }
    return nodes;
  }

  function applyFindHighlights() {
    if ("highlights" in CSS) {
      // MANDATORY: Clear previous highlights before registering new ones.
      CSS.highlights.clear();
      if (currentFindMatches.length > 0) {
        CSS.highlights.set("find-results", new Highlight(...currentFindMatches));
        if (currentFindIndex >= 0 && currentFindMatches[currentFindIndex]) {
          CSS.highlights.set("find-current", new Highlight(currentFindMatches[currentFindIndex]));
        }
      }
    } else if (fallbackMarks.length > 0) {
      fallbackMarks.forEach((m, idx) => m.classList.toggle("active", idx === currentFindIndex));
    }

    const countEl = document.getElementById("md-find-count");
    if (countEl) {
      countEl.textContent = currentFindMatches.length > 0 ? `${currentFindIndex + 1}/${currentFindMatches.length}` : "0/0";
    }
    if (currentFindIndex >= 0) {
      const target = currentFindMatches[currentFindIndex];
      const anchorEl = target?.startContainer?.parentElement || fallbackMarks[currentFindIndex];
      anchorEl?.scrollIntoView?.({ block: "center", behavior: "smooth" });
    }
  }

  function runFindQuery(query) {
    clearFindHighlights();
    const rootEl = currentFindRoot || document.getElementById("md-viewer-content");
    const q = String(query || "").trim().toLowerCase();
    if (!rootEl || !q) {
      const countEl = document.getElementById("md-find-count");
      if (countEl) countEl.textContent = "0/0";
      return;
    }

    const nodes = collectTextNodes(rootEl);
    for (const node of nodes) {
      const lower = node.nodeValue.toLowerCase();
      let pos = 0;
      while ((pos = lower.indexOf(q, pos)) !== -1) {
        const range = document.createRange();
        range.setStart(node, pos);
        range.setEnd(node, pos + q.length);
        currentFindMatches.push(range);
        pos += q.length;
      }
    }

    if (!("highlights" in CSS) && currentFindMatches.length > 0) {
      // Old browser fallback using Range.surroundContents backwards so offsets stay valid
      for (let i = currentFindMatches.length - 1; i >= 0; i--) {
        try {
          const mark = document.createElement("mark");
          mark.className = "md-find-mark";
          currentFindMatches[i].surroundContents(mark);
          fallbackMarks.unshift(mark);
        } catch (_) {}
      }
      currentFindMatches = fallbackMarks;
    }

    if (currentFindMatches.length > 0) {
      currentFindIndex = 0;
    }
    applyFindHighlights();
  }

  function stepFindMatch(delta) {
    if (!currentFindMatches.length) return;
    currentFindIndex = (currentFindIndex + delta + currentFindMatches.length) % currentFindMatches.length;
    applyFindHighlights();
  }

  function openFindPopover(rootEl) {
    currentFindRoot = rootEl || document.getElementById("md-viewer-content");
    const pop = document.getElementById("md-find-dialog");
    const input = document.getElementById("md-find-input");
    if (!pop) return;
    if (typeof pop.showPopover === "function") {
      try {
        if (!pop.matches(":popover-open")) pop.showPopover();
      } catch (_) {
        pop.classList.add("open");
      }
    } else {
      pop.classList.add("open");
    }
    setTimeout(() => input?.focus(), 40);
  }

  function closeFindPopover() {
    const pop = document.getElementById("md-find-dialog");
    if (pop) {
      if (typeof pop.hidePopover === "function") {
        try {
          if (pop.matches(":popover-open")) pop.hidePopover();
        } catch (_) {}
      }
      pop.classList.remove("open");
    }
    clearFindHighlights();
  }

  function initFindInPage() {
    document.getElementById("btn-md-viewer-find")?.addEventListener("click", () => {
      openFindPopover(document.getElementById("md-viewer-content"));
    });
    document.getElementById("btn-md-viewer-close")?.addEventListener("click", () => {
      closeFindPopover();
      window.closeMarkdownViewer?.();
    });
    document.getElementById("btn-md-find-close")?.addEventListener("click", closeFindPopover);
    document.getElementById("btn-md-find-prev")?.addEventListener("click", () => stepFindMatch(-1));
    document.getElementById("btn-md-find-next")?.addEventListener("click", () => stepFindMatch(1));
    document.getElementById("md-find-input")?.addEventListener("input", (e) => runFindQuery(e.target.value));
    document.getElementById("md-find-input")?.addEventListener("keydown", (e) => {
      if (e.key === "Enter") {
        e.preventDefault();
        stepFindMatch(e.shiftKey ? -1 : 1);
      } else if (e.key === "Escape") {
        closeFindPopover();
      }
    });
  }

  // --- 7. 200 MB Audio/Video/.amr & PDF/Code Preview Modal ---
  function openMediaPreviewModal(uri, title) {
    const dlg = document.getElementById("dialog-media-preview");
    const bodyEl = document.getElementById("media-dialog-body");
    const titleEl = document.getElementById("media-dialog-title");
    const extLink = document.getElementById("media-dialog-open-ext");
    const findBtn = document.getElementById("btn-media-dialog-find");
    if (!dlg || !bodyEl) return false;

    const ext = fileExt(uri || title);
    const rawUrl = typeof window.resolveMediaRawUrl === "function" ? window.resolveMediaRawUrl(uri) : `/gateway/files/raw?uri=${encodeURIComponent(uri)}`;

    if (titleEl) titleEl.textContent = title || (uri.split("/").pop() || "文件预览");
    if (extLink) extLink.href = rawUrl;
    findBtn?.classList.add("hidden");

    if (AUDIO_EXTS.has(ext)) {
      const amrHint = ext === "amr"
        ? `<div style="font-size:12px;color:var(--ios-secondary-label);margin-top:8px;">若当前浏览器内核不支持直接解码 .amr，请点下方链接下载播放；网关已完整支持 200 MB 音频上传与识别。</div>`
        : "";
      bodyEl.innerHTML = `<audio class="v233-audio-player" controls autoplay src="${escapeHtmlLocal(rawUrl)}"></audio>${amrHint}`;
    } else if (VIDEO_EXTS.has(ext)) {
      bodyEl.innerHTML = `<video controls playsinline style="width:100%;max-height:58dvh;border-radius:10px;background:#000;" src="${escapeHtmlLocal(rawUrl)}"></video>`;
    } else if (ext === "pdf") {
      bodyEl.innerHTML = `<iframe loading="lazy" title="PDF Preview" src="${escapeHtmlLocal(rawUrl)}" style="width:100%;height:62dvh;border:none;border-radius:10px;background:#fff;"></iframe>`;
    } else {
      return false;
    }

    if (typeof dlg.showModal === "function") dlg.showModal();
    return true;
  }

  function initMediaDialog() {
    const dlg = document.getElementById("dialog-media-preview");
    document.getElementById("btn-media-dialog-close")?.addEventListener("click", () => {
      const body = document.getElementById("media-dialog-body");
      if (body) body.innerHTML = "";
      dlg?.close();
    });
    dlg?.addEventListener("close", () => {
      const body = document.getElementById("media-dialog-body");
      if (body) body.innerHTML = "";
    });
    document.getElementById("messages-stream")?.addEventListener("click", (e) => {
      const attCard = e.target.closest('[data-action="open-attachment-file"]');
      if (attCard) {
        const uri = attCard.getAttribute("data-uri") || "";
        const name = attCard.getAttribute("data-name") || "";
        const ext = fileExt(name || uri);
        if (AUDIO_EXTS.has(ext) || VIDEO_EXTS.has(ext) || ext === "pdf") {
          e.preventDefault();
          e.stopPropagation();
          openMediaPreviewModal(uri, name || uri);
          return;
        }
        if (ext === "r" || ext === "rmd" || ext === "rmarkdown" || ext === "rprofile") {
          e.preventDefault();
          e.stopPropagation();
          window.openMarkdownViewer?.(uri, name || uri);
          return;
        }
      }
      const prBtn = e.target.closest(".v233-pr-pill");
      if (prBtn) {
        const u = decodeURIComponent(prBtn.getAttribute("data-pr-url") || "");
        const pr = prCache.get(u);
        if (pr) openPRDetailModal(pr);
        return;
      }
      const link = e.target.closest("a[href]");
      if (link && !link.classList.contains("markdown-file-link")) {
        const href = link.getAttribute("href") || "";
        const ext = fileExt(href);
        if (AUDIO_EXTS.has(ext) || VIDEO_EXTS.has(ext) || ext === "pdf") {
          e.preventDefault();
          openMediaPreviewModal(href, link.textContent.trim());
        }
      }
    }, true);
    document.getElementById("btn-pr-dialog-close")?.addEventListener("click", () => {
      document.getElementById("dialog-github-pr")?.close();
    });
  }

  function init() {
    initThemeSettings();
    installRHighlighter();
    initTrustBanner();
    initBtwDialog();
    initFindInPage();
    initMediaDialog();
  }

  window.V233 = {
    AUDIO_EXTS,
    VIDEO_EXTS,
    R_LANGS,
    LIGHT_PRESETS,
    DARK_PRESETS,
    applyTheme,
    setLightPreset,
    setDarkPreset,
    openSettingsSubpage,
    isAudioOrVideoFile,
    maxUploadBytesForFile,
    hydrateChatPRPills,
    checkWorkspaceTrust,
    openBtwModal,
    openFindPopover,
    closeFindPopover,
    openMediaPreviewModal,
  };

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
