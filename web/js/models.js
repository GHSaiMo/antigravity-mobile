// 动态模型目录与常用模型（常用模型 1 / 常用模型 2 + 长按全模型二级选单）。
//
// 网关 GET /gateway/models 返回 language_server 当前可用的模型（含内部枚举）和建议默认模型；
// 设置页配置两个常用模型（均支持全部模型与全部思考强度），新会话首选常用模型 1；
// 单击模型胶囊在两个常用模型间快速切换；长按/右键列出全部模型（Gemini 蓝 / Claude 橙 / Orchestrator 钛灰）及二级思考强度（Low -> Medium -> High -> Extra，默认 Medium）。

const MODEL_FALLBACK = {
  gemini: "gemini-4-neon-high",
  claude: "claude-opus-4-6-thinking",
  orchestrator: "gemini-pro-orchestrator-neon"
};
const MODEL_ENUM_FALLBACK = {
  gemini: "MODEL_PLACEHOLDER_M532",
  claude: "MODEL_PLACEHOLDER_M26",
  orchestrator: "MODEL_PLACEHOLDER_M537"
};
const MODEL_CATALOG_KEY = "agy_model_catalog";
const MODEL_DEFAULT_KEY = (provider) => `agy_default_${provider}_model`;
const FAVORITE_MODEL_KEY = (slot) => `agy_favorite_model_${slot}`;
const IGNORED_PICKER_TAGS = new Set(["new", "fast", "notice", "提示"]);
const CLAUDE_ENUM_TOKENS = ["m26", "m35", "m400", "m401", "m402", "m403", "m404", "m405"];
const ORCHESTRATOR_ENUM_TOKENS = ["m537", "m667", "m681"];
const MODEL_FALLBACK_ORDER = [
  "gemini-4-neon-high", "gemini-4-neon-xhigh", "gemini-4-neon-medium", "gemini-4-neon-low",
  "gemini-4-carbon-medium", "gemini-4-carbon-high", "gemini-4-carbon-low",
  "gemini-4-argon-medium", "gemini-4-argon-high", "gemini-4-argon-low",
  "gemini-4-hydrogen-xhigh", "gemini-4-hydrogen-high", "gemini-4-hydrogen-medium", "gemini-4-hydrogen-low",
  "gemini-3.8-flash-high", "gemini-3.8-flash-medium", "gemini-3.8-flash-low",
  "claude-opus-5-5-high", "claude-opus-5-5-medium", "claude-opus-5-5-low",
  "claude-sonnet-5-5-high", "claude-sonnet-5-5-medium", "claude-sonnet-5-5-low",
  "claude-opus-4-6-thinking", "claude-sonnet-4-6",
  "gemini-pro-orchestrator-neon", "gemini-pro-orchestrator", "gemini-pro-orchestrator-argon"
];
const MODEL_STATIC_NAMES = {
  "gemini-4-neon-xhigh": "Gemini 4 Neon (XHigh)",
  "gemini-4-neon-high": "Gemini 4 Neon (High)",
  "gemini-4-neon-medium": "Gemini 4 Neon (Medium)",
  "gemini-4-neon-low": "Gemini 4 Neon (Low)",
  "gemini-4-neon": "Gemini 4 Neon",
  "gemini-4-hydrogen-xhigh": "Gemini 4 Hydrogen (XHigh)",
  "gemini-4-hydrogen-high": "Gemini 4 Hydrogen (High)",
  "gemini-4-hydrogen-medium": "Gemini 4 Hydrogen (Medium)",
  "gemini-4-hydrogen-low": "Gemini 4 Hydrogen (Low)",
  "gemini-4-carbon-high": "Gemini 4 Carbon (High)",
  "gemini-4-carbon-medium": "Gemini 4 Carbon (Medium)",
  "gemini-4-carbon-low": "Gemini 4 Carbon (Low)",
  "gemini-4-carbon": "Gemini 4 Carbon",
  "gemini-4-argon-high": "Gemini 4 Argon (High)",
  "gemini-4-argon-medium": "Gemini 4 Argon (Medium)",
  "gemini-4-argon-low": "Gemini 4 Argon (Low)",
  "gemini-3.8-flash-high": "Gemini 3.8 Flash (High)",
  "gemini-3.8-flash-medium": "Gemini 3.8 Flash (Medium)",
  "gemini-3.8-flash-low": "Gemini 3.8 Flash (Low)",
  "gemini-3.1-pro-high": "Gemini 3.1 Pro (High)",
  "gemini-3.1-pro-low": "Gemini 3.1 Pro (Low)",
  "claude-opus-5-5-high": "Claude Opus 5.5 (High)",
  "claude-opus-5-5-medium": "Claude Opus 5.5 (Medium)",
  "claude-opus-5-5-low": "Claude Opus 5.5 (Low)",
  "claude-sonnet-5-5-high": "Claude Sonnet 5.5 (High)",
  "claude-sonnet-5-5-medium": "Claude Sonnet 5.5 (Medium)",
  "claude-sonnet-5-5-low": "Claude Sonnet 5.5 (Low)",
  "claude-opus-4-6-thinking": "Claude Opus 4.6 (Thinking)",
  "claude-sonnet-4-6": "Claude Sonnet 4.6 (Thinking)",
  "gemini-pro-orchestrator-neon": "Orchestrator (Neon)",
  "orchestrator-neon": "Orchestrator (Neon)",
  "gemini-pro-orchestrator": "Orchestrator (Carbon)",
  "gemini-pro-orchestrator-argon": "Orchestrator (Argon)",
  "orchestrator-carbon": "Orchestrator (Carbon)",
  "orchestrator-argon": "Orchestrator (Argon)",
  "gemini-flash-worker-agy": "Gemini 4 Hydrogen (Orchestrator)",
  "gpt-oss-120b-medium": "GPT-OSS 120B (Medium)",
};

let modelCatalog = { models: [], defaults: {}, live: false };

function isClaudeModel(id) {
  const lower = String(id || "").toLowerCase();
  if (lower.includes("claude")) return true;
  return CLAUDE_ENUM_TOKENS.some((tok) => lower.includes(tok));
}

function modelProviderKind(id) {
  const lower = String(id || "").toLowerCase();
  if (isClaudeModel(lower)) return "claude";
  if (lower.includes("orch") || ORCHESTRATOR_ENUM_TOKENS.some((tok) => lower.includes(tok))) return "orchestrator";
  if (lower.includes("gpt")) return "other";
  return "gemini";
}

// 输入框上方胶囊的厂商名
function modelProviderLabel(id) {
  switch (modelProviderKind(id)) {
    case "claude": return "Claude";
    case "orchestrator": return "Orchestrator";
    case "other": return "GPT";
    default: return "Gemini";
  }
}

function normalizeDisplayName(raw) {
  return String(raw || "")
    .trim()
    .replace(/(\(|[\s,\-·/|:]\s*)(xhigh|extra(?:[\s\-]+high)?)\)/gi, "$1XHigh)");
}

function parsePureEffortRank(raw) {
  const s = String(raw || "").trim().toLowerCase();
  if (s === "xhigh" || s === "extra" || s === "extra high" || s === "extra-high") return 3;
  if (s === "high") return 2;
  if (s === "medium" || s === "med") return 1;
  if (s === "low") return 0;
  return -1;
}

function extractTrailingEffort(raw) {
  const trimmed = String(raw || "").trim();
  if (!trimmed) return null;
  const pure = parsePureEffortRank(trimmed);
  if (pure >= 0) return { base: "", rank: pure };
  const m = trimmed.match(/^(.*?)[\s,\-·/|:–—]+(extra[\s\-]+high|xhigh|extra|medium|med|high|low)$/i);
  if (!m) return null;
  const rank = parsePureEffortRank(m[2]);
  if (rank < 0) return null;
  return { base: m[1].trim(), rank };
}

function cleanOrchestratorModelPart(raw) {
  let s = String(raw || "").trim().replace(/^[\s\-·,/:|–—()]+|[\s\-·,/:|–—()]+$/g, "");
  const lower = s.toLowerCase();
  if (lower.startsWith("gemini 4 ")) s = s.slice("gemini 4 ".length);
  else if (lower.startsWith("gemini ")) s = s.slice("gemini ".length);
  else if (lower.startsWith("claude ")) s = s.slice("claude ".length);
  return s.replace(/^[\s\-·,/:|–—()]+|[\s\-·,/:|–—()]+$/g, "");
}

function parseOrchestratorName(fullName, id = "") {
  const normalized = normalizeDisplayName(fullName);
  const stripOrch = (text) => {
    let s = String(text || "").trim();
    if (/^orchestrator\b/i.test(s)) s = s.slice("orchestrator".length).trim();
    if (/\(orchestrator\)$/i.test(s)) s = s.slice(0, -"(orchestrator)".length).trim();
    else if (/\borchestrator$/i.test(s)) s = s.slice(0, -"orchestrator".length).trim();
    return cleanOrchestratorModelPart(s);
  };
  if (!normalized) {
    return { modelName: "", effortRank: parseEffortRank(id) };
  }
  const m = normalized.match(/^(.+?)\s*\(([^()]+)\)$/);
  if (m) {
    const prefix = m[1].trim();
    const inside = m[2].trim();
    const pureEffort = parsePureEffortRank(inside);
    if (pureEffort >= 0) {
      const pm = prefix.match(/^(.+?)\s*\(([^()]+)\)$/);
      if (pm) {
        const pBefore = pm[1].trim();
        const pInner = pm[2].trim();
        if (/^orchestrator\b/i.test(pBefore)) {
          return { modelName: cleanOrchestratorModelPart(pInner), effortRank: pureEffort };
        }
        if (/^orchestrator$/i.test(pInner)) {
          return { modelName: cleanOrchestratorModelPart(pBefore), effortRank: pureEffort };
        }
      }
      return { modelName: stripOrch(prefix), effortRank: pureEffort };
    }
    const split = extractTrailingEffort(inside);
    if (split) {
      if (/^orchestrator\b/i.test(prefix)) {
        return { modelName: cleanOrchestratorModelPart(split.base), effortRank: split.rank };
      }
      if (/^orchestrator$/i.test(split.base)) {
        return { modelName: cleanOrchestratorModelPart(prefix), effortRank: split.rank };
      }
      return { modelName: cleanOrchestratorModelPart(split.base), effortRank: split.rank };
    }
    const idRank = parseEffortRank(id);
    if (/^orchestrator\b/i.test(prefix)) {
      return { modelName: cleanOrchestratorModelPart(inside), effortRank: idRank };
    }
    if (/^orchestrator$/i.test(inside)) {
      return { modelName: cleanOrchestratorModelPart(prefix), effortRank: idRank };
    }
    return { modelName: stripOrch(normalized), effortRank: idRank };
  }
  const split = extractTrailingEffort(normalized);
  if (split) {
    return { modelName: stripOrch(split.base), effortRank: split.rank };
  }
  return { modelName: stripOrch(normalized), effortRank: parseEffortRank(id) };
}

function formatOrchestratorSlidePickerTitle(rawTitle) {
  const { modelName } = parseOrchestratorName(rawTitle);
  return modelName ? `Orch ${modelName}` : "Orch";
}

function formatModelIDAsDisplayName(id) {
  const trimmed = String(id || "").trim();
  if (!trimmed) return "";
  const staticHit = MODEL_STATIC_NAMES[trimmed.toLowerCase()];
  if (staticHit) return staticHit;
  const tokens = trimmed.split("-").filter(Boolean);
  if (!tokens.length) return trimmed;
  let qualifier = null;
  if (tokens.length >= 2 && tokens[tokens.length - 2].toLowerCase() === "extra" && tokens[tokens.length - 1].toLowerCase() === "high") {
    tokens.pop();
    tokens.pop();
    qualifier = "XHigh";
  } else {
    const qualifiers = new Set(["xhigh", "extra", "high", "medium", "low", "thinking"]);
    if (tokens.length > 1 && qualifiers.has(tokens[tokens.length - 1].toLowerCase())) {
      const q = tokens.pop().toLowerCase();
      qualifier = (q === "xhigh" || q === "extra") ? "XHigh" : (q.charAt(0).toUpperCase() + q.slice(1));
    }
  }
  const lowerTokens = tokens.map((t) => t.toLowerCase());
  const isGeminiProOrch = lowerTokens.length >= 3 && lowerTokens[0] === "gemini" && lowerTokens[1] === "pro" && lowerTokens[2] === "orchestrator";
  if (isGeminiProOrch || lowerTokens[0] === "orchestrator") {
    const rest = isGeminiProOrch ? tokens.slice(3) : tokens.slice(1);
    const modelName = rest.length
      ? rest.map((tok) => (/^[a-z]/i.test(tok) ? tok.charAt(0).toUpperCase() + tok.slice(1) : tok)).join(" ")
      : (isGeminiProOrch ? "Carbon" : "");
    const base = modelName ? `Orchestrator (${modelName})` : "Orchestrator";
    return qualifier ? `${base} (${qualifier})` : base;
  }
  const parts = [];
  for (const tok of tokens) {
    if (/^\d+$/.test(tok) && parts.length && /^[\d.]+$/.test(parts[parts.length - 1])) {
      parts[parts.length - 1] += `.${tok}`;
    } else if (/^(gpt|oss)$/i.test(tok)) {
      parts.push(tok.toUpperCase());
    } else if (/^[a-z]/i.test(tok)) {
      parts.push(tok.charAt(0).toUpperCase() + tok.slice(1));
    } else {
      parts.push(tok);
    }
  }
  const base = parts.join(" ");
  return qualifier ? `${base} (${qualifier})` : base;
}

function formatPickerLabel(name, tag) {
  const cleanName = normalizeDisplayName(name);
  const cleanTag = String(tag || "").trim();
  if (cleanTag && !IGNORED_PICKER_TAGS.has(cleanTag.toLowerCase())) {
    return `${cleanName} · ${cleanTag}`;
  }
  return cleanName;
}

function modelDisplayName(id) {
  const m = modelCatalog.models.find((x) => x.id === id);
  return (m && m.name) ? normalizeDisplayName(m.name) : formatModelIDAsDisplayName(id);
}

function formatFullNameToCompact(full) {
  const trimmed = normalizeDisplayName(full);
  if (!trimmed) return "";
  if (/^orchestrator\b/i.test(trimmed)) {
    const { modelName, effortRank } = parseOrchestratorName(trimmed);
    const base = modelName ? `Orch ${modelName}` : "Orch";
    if (effortRank >= 0) {
      const shortEffort = ["Low", "Med", "High", "XHigh"][effortRank];
      return `${base} · ${shortEffort}`;
    }
    return base;
  }
  const match = trimmed.match(/^(.+?)\s*\(([^)]+)\)$/);
  if (match) {
    let base = match[1].trim();
    let variant = match[2].trim();
    const bl = base.toLowerCase();
    if (bl.startsWith("gemini 4 ")) base = base.slice("gemini 4 ".length);
    else if (bl.startsWith("gemini ")) base = base.slice("gemini ".length);
    else if (bl.startsWith("claude ")) base = base.slice("claude ".length);
    else if (bl.startsWith("orchestrator")) base = "Orch";
    const vl = variant.toLowerCase();
    if (vl === "medium") variant = "Med";
    else if (vl === "thinking") variant = "Think";
    else if (vl === "xhigh" || vl === "extra" || vl === "extra high" || vl === "extra-high") variant = "XHigh";
    return `${base} · ${variant}`;
  }
  let base = trimmed;
  const bl = base.toLowerCase();
  if (bl.startsWith("gemini 4 ")) base = base.slice("gemini 4 ".length);
  else if (bl.startsWith("gemini ")) base = base.slice("gemini ".length);
  else if (bl.startsWith("claude ")) base = base.slice("claude ".length);
  else if (bl.startsWith("orchestrator")) base = "Orch";
  return base;
}

function compactModelDisplayName(id) {
  return formatFullNameToCompact(modelDisplayName(id));
}

function providerSortRank(p) {
  switch (String(p || "").toLowerCase()) {
    case "gemini": return 0;
    case "claude": return 1;
    case "orchestrator": return 2;
    default: return 3;
  }
}

function isExcludedPickerModelId(id) {
  const lower = String(id || "").trim().toLowerCase();
  return lower.includes("gpt-oss") || lower.endsWith("-disabled");
}

function allSortedModelOptions() {
  const filtered = (modelCatalog.models || []).filter((m) => !isExcludedPickerModelId(m.id));
  const raw = filtered.length > 0
    ? filtered
    : MODEL_FALLBACK_ORDER.map((id) => ({ id, name: MODEL_STATIC_NAMES[id], provider: modelProviderKind(id) }));
  return raw.map((opt, index) => ({ opt, index })).sort((a, b) => {
    const ka = modelProviderKind(a.opt.id);
    const kb = modelProviderKind(b.opt.id);
    const pa = (ka === "orchestrator" || !a.opt.provider) ? ka : a.opt.provider.toLowerCase();
    const pb = (kb === "orchestrator" || !b.opt.provider) ? kb : b.opt.provider.toLowerCase();
    const ra = providerSortRank(pa);
    const rb = providerSortRank(pb);
    if (ra !== rb) return ra - rb;
    return a.index - b.index;
  }).map((x) => x.opt);
}

function parseEffortRank(labelOrId) {
  const pure = parsePureEffortRank(labelOrId);
  if (pure >= 0) return pure;
  const s = String(labelOrId || "").trim().toLowerCase();
  if (s.endsWith("-xhigh") || s.endsWith("-extra-high") || s.endsWith("-extra")) return 3;
  if (s.endsWith("-high")) return 2;
  if (s.endsWith("-medium")) return 1;
  if (s.endsWith("-low")) return 0;
  return -1;
}

function effortDisplayLabel(label, id) {
  const r = parseEffortRank(label) >= 0 ? parseEffortRank(label) : parseEffortRank(id);
  return ["Low", "Medium", "High", "XHigh"][r] || label;
}

function effortShortLabel(label, id) {
  const r = parseEffortRank(label) >= 0 ? parseEffortRank(label) : parseEffortRank(id);
  return ["Low", "Med", "High", "XHigh"][r] || label;
}

function modelEffortRank(id) {
  const direct = parseEffortRank(id);
  if (direct >= 0) return direct;
  const full = modelDisplayName(id);
  const m = full.match(/\(([^()]+)\)$/);
  if (m) {
    const pure = parsePureEffortRank(m[1]);
    if (pure >= 0) return pure;
    const split = extractTrailingEffort(m[1]);
    if (split) return split.rank;
  }
  return -1;
}

function buildModelFamilies() {
  const opts = allSortedModelOptions();
  const map = new Map();
  const order = [];
  for (const opt of opts) {
    const fullName = opt.name ? normalizeDisplayName(opt.name) : modelDisplayName(opt.id);
    const kind = modelProviderKind(opt.id);
    const optProv = String(opt.provider || "").toLowerCase();
    const prov = (kind === "orchestrator" || optProv === "orchestrator" || /^orchestrator\b/i.test(fullName))
      ? "orchestrator"
      : (optProv || kind);

    let familyName = fullName;
    let variantLabel = "默认";
    if (prov === "orchestrator") {
      const { modelName, effortRank } = parseOrchestratorName(fullName, opt.id);
      familyName = modelName ? `Orchestrator (${modelName})` : "Orchestrator";
      if (effortRank >= 0) {
        variantLabel = ["Low", "Medium", "High", "XHigh"][effortRank];
      }
    } else {
      const m = fullName.match(/^(.+?)\s*\(([^)]+)\)$/);
      if (m) {
        familyName = m[1].trim();
        variantLabel = m[2].trim();
      }
    }
    const variant = { id: opt.id, label: variantLabel, rawName: fullName, tag: opt.tag || null, quotaPool: opt.quotaPool || null };
    if (!map.has(familyName)) {
      order.push(familyName);
      map.set(familyName, { id: familyName, name: familyName, provider: prov, variants: [variant] });
    } else {
      map.get(familyName).variants.push(variant);
    }
  }
  const families = order.map((k) => {
    const fam = map.get(k);
    const variantRank = (v) => (parseEffortRank(v.label) >= 0 ? parseEffortRank(v.label) : parseEffortRank(v.id));
    const hasEffort = fam.variants.length > 1 && fam.variants.every((v) => variantRank(v) >= 0);
    const hasAnyEffort = fam.variants.length > 1 && fam.variants.some((v) => variantRank(v) >= 0);
    const ordered = hasAnyEffort
      ? [...fam.variants].sort((a, b) => {
          const ra = variantRank(a) >= 0 ? variantRank(a) : Number.MAX_SAFE_INTEGER;
          const rb = variantRank(b) >= 0 ? variantRank(b) : Number.MAX_SAFE_INTEGER;
          return ra - rb; // Low -> Medium -> High -> Extra
        })
      : fam.variants;
    const medIdx = ordered.findIndex((v) => variantRank(v) === 1);
    const highIdx = ordered.findIndex((v) => variantRank(v) === 2);
    const extraIdx = ordered.findIndex((v) => variantRank(v) === 3);
    const defaultIdx = fam.provider === "orchestrator"
      ? (highIdx >= 0 ? highIdx : (medIdx >= 0 ? medIdx : (extraIdx >= 0 ? extraIdx : 0)))
      : (medIdx >= 0 ? medIdx : (highIdx >= 0 ? highIdx : 0));
    return {
      ...fam,
      hasEffortVariants: hasEffort,
      hasAnyEffortVariants: hasAnyEffort,
      variantsTopToBottom: ordered,
      defaultVariantIndex: defaultIdx
    };
  });
  return families.map((f, idx) => ({ f, idx })).sort((a, b) => {
    const r = providerSortRank(a.f.provider) - providerSortRank(b.f.provider);
    return r !== 0 ? r : a.idx - b.idx;
  }).map((x) => x.f);
}

// 常用模型读写（自动从旧版 default_gemini / default_claude 迁移）
function getFavoriteModel(slot) {
  let saved = "";
  try {
    saved = localStorage.getItem(FAVORITE_MODEL_KEY(slot)) || "";
    if (!saved) {
      const legacyKey = slot === 1 ? MODEL_DEFAULT_KEY("gemini") : MODEL_DEFAULT_KEY("claude");
      saved = localStorage.getItem(legacyKey) || "";
    }
  } catch (_) {}
  const fallback = slot === 1 ? MODEL_FALLBACK.gemini : MODEL_FALLBACK.claude;
  const resolved = saved || fallback;
  return validatedFavoriteModel(resolved, fallback, slot === 2 ? getFavoriteModelSafeRaw(1) : null);
}

function getFavoriteModelSafeRaw(slot) {
  try {
    return localStorage.getItem(FAVORITE_MODEL_KEY(slot)) || (slot === 1 ? localStorage.getItem(MODEL_DEFAULT_KEY("gemini")) : localStorage.getItem(MODEL_DEFAULT_KEY("claude"))) || (slot === 1 ? MODEL_FALLBACK.gemini : MODEL_FALLBACK.claude);
  } catch (_) {
    return slot === 1 ? MODEL_FALLBACK.gemini : MODEL_FALLBACK.claude;
  }
}

function setFavoriteModel(slot, id) {
  try {
    localStorage.setItem(FAVORITE_MODEL_KEY(slot), id);
  } catch (_) {}
}

function validatedFavoriteModel(saved, fallback, avoid = null) {
  const selectable = (modelCatalog.models || []).filter((m) => !isExcludedPickerModelId(m.id));
  if (!modelCatalog.live || !selectable.length) return saved;
  if (selectable.some((m) => m.id === saved)) return saved;
  const differs = (id) => !avoid || id.toLowerCase() !== avoid.toLowerCase();
  const savedKind = modelProviderKind(saved);
  const provDefault = modelCatalog.defaults[savedKind];
  if (provDefault && selectable.some((m) => m.id === provDefault) && differs(provDefault)) return provDefault;
  if (selectable.some((m) => m.id === fallback) && differs(fallback)) return fallback;
  const other = selectable.find((m) => differs(m.id));
  return other ? other.id : (selectable[0]?.id || saved);
}

function getDefaultModel(provider) {
  return provider === "claude" ? getFavoriteModel(2) : getFavoriteModel(1);
}

// 单击胶囊在两个常用模型间快切（当前是第 3 个模型时回切常用模型 1）
function modelToggleTarget(current) {
  const fav1 = getFavoriteModel(1);
  const fav2 = getFavoriteModel(2);
  if (!fav1) return fav2 || current;
  if (!fav2) return fav1;
  return String(current || "").toLowerCase() === fav1.toLowerCase() ? fav2 : fav1;
}

// language_server 里切换默认模型用的内部枚举
function modelEnumFor(id) {
  const m = modelCatalog.models.find((x) => x.id === id);
  if (m && m.enum) return m.enum;
  const kind = modelProviderKind(id);
  return MODEL_ENUM_FALLBACK[kind] || MODEL_ENUM_FALLBACK.gemini;
}

function resolveActiveModel(raw, current) {
  const value = String(raw || "").trim();
  if (!value) return current;
  if (value.toUpperCase().startsWith("MODEL_")) {
    const m = modelCatalog.models.find((x) => x.enum === value);
    if (m) return m.id;
    return modelProviderKind(value) === "claude" ? getFavoriteModel(2) : getFavoriteModel(1);
  }
  return value;
}

function applyModelCatalog(data) {
  if (!data || !Array.isArray(data.models) || data.models.length === 0) return;
  modelCatalog = { models: data.models, defaults: data.defaults || {}, live: data.live === true };
  if (modelCatalog.live) {
    const f1 = validatedFavoriteModel(getFavoriteModelSafeRaw(1), MODEL_FALLBACK.gemini, null);
    setFavoriteModel(1, f1);
    const f2 = validatedFavoriteModel(getFavoriteModelSafeRaw(2), MODEL_FALLBACK.claude, f1);
    setFavoriteModel(2, f2);
  }
  if (typeof updateModelSwitchUI === "function") updateModelSwitchUI();
  renderFavoriteModelSettings();
}

async function loadModelCatalog(force = false) {
  if (!force) {
    try {
      const cached = JSON.parse(localStorage.getItem(MODEL_CATALOG_KEY) || "null");
      if (cached) applyModelCatalog(cached);
    } catch (_) {}
  }
  try {
    const resp = await fetch(force ? "/gateway/models?refresh=1" : "/gateway/models");
    if (!resp.ok) return;
    const data = await resp.json();
    applyModelCatalog(data);
    try {
      localStorage.setItem(MODEL_CATALOG_KEY, JSON.stringify(data));
    } catch (_) {}
  } catch (_) {}
}

function renderFavoriteModelSettings() {
  const options = allSortedModelOptions();
  const groupDefs = [
    { key: "gemini", title: "Gemini" },
    { key: "claude", title: "Claude" },
    { key: "orchestrator", title: "Orchestrator" },
    { key: "other", title: "其他模型" }
  ];
  for (const slot of [1, 2]) {
    const select = document.getElementById(`settings-favorite-${slot}`);
    if (!select) continue;
    const current = getFavoriteModel(slot);
    const hasCurrent = options.some((m) => m.id === current);
    const parts = [];
    if (!hasCurrent && current) {
      parts.push(`<option value="${escapeHtml(current)}">${escapeHtml(modelDisplayName(current))}</option>`);
    }
    for (const grp of groupDefs) {
      const items = options.filter((m) => {
        const k = modelProviderKind(m.id);
        const resolved = (k === "orchestrator" || !m.provider) ? k : m.provider.toLowerCase();
        return grp.key === "other" ? !["gemini", "claude", "orchestrator"].includes(resolved) : resolved === grp.key;
      });
      if (!items.length) continue;
      const optsHtml = items.map((m) => {
        const label = formatPickerLabel(m.name || modelDisplayName(m.id), m.tag);
        return `<option value="${escapeHtml(m.id)}">${escapeHtml(label)}</option>`;
      }).join("");
      parts.push(`<optgroup label="${grp.title}">${optsHtml}</optgroup>`);
    }
    select.innerHTML = parts.join("");
    select.value = current;
  }
  const note = document.getElementById("settings-models-note");
  if (note) {
    note.textContent = modelCatalog.live
      ? "新会话默认使用首选模型，已有会话自动同步电脑端最新模型；短按模型胶囊切换首选与次选，长按可任选模型。"
      : "当前显示内置模型列表；新会话默认使用首选模型，已有会话自动同步电脑端最新模型；短按模型胶囊切换首选与次选，长按可任选模型。";
  }
}

function initModelSettings() {
  for (const slot of [1, 2]) {
    document.getElementById(`settings-favorite-${slot}`)?.addEventListener("change", (e) => {
      setFavoriteModel(slot, e.target.value);
      if (typeof updateModelSwitchUI === "function") updateModelSwitchUI();
    });
  }
  document.getElementById("btn-refresh-models")?.addEventListener("click", async (e) => {
    const btn = e.currentTarget;
    const label = btn.querySelector("span");
    const origText = label ? label.textContent : "";
    btn.disabled = true;
    if (label) label.textContent = "正在刷新...";
    try {
      await loadModelCatalog(true);
      if (label) label.textContent = modelCatalog.live ? "已刷新" : "刷新失败，仍为内置列表";
    } finally {
      setTimeout(() => {
        btn.disabled = false;
        if (label) label.textContent = origText || "刷新模型列表";
      }, 1200);
    }
  });
  renderFavoriteModelSettings();
  loadModelCatalog();
}

// Web 端长按上滑 / 右键全模型单列行内强度选择器（蓝色 Gemini / 橙色 Claude / 石墨钛灰 Orchestrator；行内横滑 Low -> Med -> High -> XHigh，右侧伴随展示 Gemini/Claude 双池 5h + 周额度）
let webPickerState = null;

function dismissWebModelPicker() {
  const overlay = document.getElementById("model-picker-overlay");
  if (overlay) {
    overlay.classList.add("hidden");
    overlay.innerHTML = "";
  }
  webPickerState = null;
}

// --- 模型选择器右侧额度伴随卡（默认开启，同屏展示 Gemini / Claude 的 5h 与周额度） ---
const MODEL_PICKER_QUOTA_ENABLED_KEY = "mgy.modelPickerQuotaEnabled";
const MODEL_PICKER_QUOTA_WEEKLY_KEY = "mgy.modelPickerQuotaWeekly";

function isModelPickerQuotaEnabled() {
  try { return localStorage.getItem(MODEL_PICKER_QUOTA_ENABLED_KEY) !== "0"; } catch (_) { return true; }
}

function isModelPickerQuotaWeekly() {
  try { return localStorage.getItem(MODEL_PICKER_QUOTA_WEEKLY_KEY) === "1"; } catch (_) { return false; }
}

const CLAUDE_ORCHESTRATOR_TOKENS = ["claude", "opus", "sonnet", "haiku", "anthropic"];

function orchestratorUsesClaudeQuotaPool(row) {
  const variants = row.variants || [];
  const explicit = variants
    .map((v) => String(v.quotaPool || "").trim().toLowerCase())
    .find(Boolean);
  if (explicit === "claude") return true;
  if (explicit === "gemini") return false;
  const titleLower = String(row.title || "").toLowerCase();
  if (CLAUDE_ORCHESTRATOR_TOKENS.some((tok) => titleLower.includes(tok))) return true;
  return variants.some((v) => {
    const idLower = String(v.id || "").toLowerCase();
    const rawLower = String(v.rawName || "").toLowerCase();
    return isClaudeModel(idLower) || CLAUDE_ORCHESTRATOR_TOKENS.some((tok) => idLower.includes(tok) || rawLower.includes(tok));
  });
}

function modelPickerQuotaPool(row) {
  const provider = String(row?.provider || "");
  if (provider === "claude") return "claude";
  if (provider === "gemini") return "gemini";
  if (provider === "orchestrator") {
    return orchestratorUsesClaudeQuotaPool(row) ? "claude" : "gemini";
  }
  return null;
}

function modelPickerQuotaPercent(row) {
  if (!isModelPickerQuotaEnabled()) return null;
  const data = typeof currentCockpitQuotas !== "undefined" ? currentCockpitQuotas : null;
  const acc = data && (data.current_account || (data.accounts && data.accounts[0]));
  if (!acc) return null;
  const pool = modelPickerQuotaPool(row);
  if (!pool) return null;
  const weekly = isModelPickerQuotaWeekly();
  const bucket = pool === "claude"
    ? (weekly ? acc.claude_weekly : (acc.claude_5h || acc.claude_weekly))
    : (weekly ? acc.gemini_weekly : (acc.gemini_5h || acc.gemini_weekly));
  if (!bucket || typeof bucket.remaining_percent !== "number") return null;
  return Math.round(Math.min(100, Math.max(0, bucket.remaining_percent)));
}

function compactQuotaResetLabel(raw) {
  const text = String(raw || "").trim();
  if (!text) return "";
  const lower = text.toLowerCase();
  if (["已就绪", "就绪", "未知", "配额充足", "ready", "unknown"].includes(lower)) return "";
  const m = text.match(/(\d+)\s*(天|日|d|小时|小時|h|分钟|分鐘|分|m)/i);
  if (!m) return "";
  const num = m[1];
  const rawUnit = m[2].toLowerCase();
  const unit = (rawUnit === "天" || rawUnit === "日" || rawUnit === "d")
    ? "d"
    : ((rawUnit === "小时" || rawUnit === "小時" || rawUnit === "h") ? "h" : "m");
  return `${num}${unit}`;
}

function effortFillPercent(index, count) {
  if (count <= 1) return 100;
  const c = Math.min(Math.max(0, index), count - 1);
  if (count === 2) return c === 0 ? 50 : 100;
  if (count === 3) return [35, 68, 100][c];
  if (count === 4) return [28, 52, 76, 100][c];
  return Math.round(24 + (76 * c) / (count - 1));
}

function effortRankKey(rank) {
  if (rank === 3) return "xhigh";
  if (rank === 2) return "high";
  if (rank === 1) return "med";
  return "low";
}

function variantEffortRank(v) {
  if (!v) return -1;
  const byLabel = parseEffortRank(v.label);
  return byLabel >= 0 ? byLabel : parseEffortRank(v.id);
}

const COSMIC_STARS_SVG = (() => {
  const stars = [
    [7, 28, 1.05, 0.2, false], [13, 72, 0.85, 1.4, false], [21, 34, 1.45, 0.7, true],
    [28, 66, 0.90, 2.1, false], [35, 22, 1.10, 0.4, false], [43, 76, 1.25, 1.8, false],
    [50, 30, 0.85, 2.5, false], [57, 62, 1.50, 1.1, true], [64, 24, 0.95, 0.5, false],
    [70, 74, 1.10, 2.0, false], [76, 38, 0.80, 1.3, false], [82, 68, 1.45, 0.9, true],
    [88, 26, 1.00, 2.3, false], [93, 54, 1.20, 1.6, false], [46, 48, 0.75, 0.3, false],
    [61, 44, 0.85, 1.9, false]
  ];
  const dots = stars.map(([x, y, r, delay, sparkle]) => {
    const cx = `${x}%`;
    const cy = `${y}%`;
    return `<span class="mp-star${sparkle ? " is-sparkle" : ""}" style="left:${cx};top:${cy};width:${(r * 2).toFixed(1)}px;height:${(r * 2).toFixed(1)}px;animation-delay:${delay}s;"></span>`;
  }).join("");
  return `<span class="mp-cosmic-stars" aria-hidden="true"><span class="mp-cosmic-sweep"></span>${dots}</span>`;
})();

function initQuotaDisplaySettings() {
  const paceToggle = document.getElementById("settings-weekly-pace-cursor");
  const toggle = document.getElementById("settings-picker-quota-enabled");
  const summary = document.getElementById("settings-quota-display-summary");
  const tr = (s) => (typeof t === "function" ? t(s) : s);
  const paceEnabled = () => (typeof isWeeklyPaceCursorEnabled === "function" ? isWeeklyPaceCursorEnabled() : true);
  const sync = () => {
    const pace = paceEnabled();
    const enabled = isModelPickerQuotaEnabled();
    if (paceToggle) paceToggle.checked = pace;
    if (toggle) toggle.checked = enabled;
    if (summary) {
      if (pace && enabled) summary.textContent = `${tr("匀速")} · ${tr("余量")}`;
      else if (pace) summary.textContent = tr("匀速游标");
      else if (enabled) summary.textContent = tr("余量面板");
      else summary.textContent = tr("已关闭");
    }
  };
  paceToggle?.addEventListener("change", () => {
    try { localStorage.setItem("mgy.weeklyPaceCursorEnabled", paceToggle.checked ? "1" : "0"); } catch (_) {}
    sync();
    if (typeof renderQuotaSheet === "function" && typeof currentCockpitQuotas !== "undefined" && currentCockpitQuotas) {
      renderQuotaSheet(currentCockpitQuotas);
    }
  });
  toggle?.addEventListener("change", () => {
    try { localStorage.setItem(MODEL_PICKER_QUOTA_ENABLED_KEY, toggle.checked ? "1" : "0"); } catch (_) {}
    sync();
  });
  window.addEventListener("mgy:languagechange", sync);
  sync();
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", initQuotaDisplaySettings);
} else {
  initQuotaDisplaySettings();
}

function resolveVariantIndexForX(row, localX, trackWidth, currentVariant, hysteresisPx = 6) {
  const count = row?.variants?.length || 0;
  if (count <= 1 || trackWidth <= 1) return 0;
  if (localX <= 0) return 0;
  if (localX >= trackWidth) return count - 1;
  const segW = trackWidth / count;
  if (currentVariant !== null && currentVariant !== undefined && currentVariant >= 0 && currentVariant < count) {
    const curMin = currentVariant * segW - hysteresisPx;
    const curMax = (currentVariant + 1) * segW + hysteresisPx;
    if (localX >= curMin && localX <= curMax) return currentVariant;
  }
  return Math.min(count - 1, Math.max(0, Math.floor(localX / segW)));
}

function buildQuotaCompanionHtml(acc) {
  if (!isModelPickerQuotaEnabled() || !acc) return "";
  const hasGemini = Boolean(acc.gemini_5h || acc.gemini_weekly);
  const hasClaude = Boolean(acc.claude_5h || acc.claude_weekly);
  if (!hasGemini && !hasClaude) return "";

  const tr = (s) => (typeof t === "function" ? t(s) : s);
  const tier = acc.subscription_tier || "";
  const isFree = String(tier).trim().toUpperCase() === "FREE" || (!acc.gemini_5h && !acc.claude_5h);
  const showPace = typeof isWeeklyPaceCursorEnabled === "function" ? isWeeklyPaceCursorEnabled() : true;

  const renderBarRow = (label, bucket, withPace) => {
    if (!bucket || typeof bucket.remaining_percent !== "number") return "";
    const pct = Math.min(100, Math.max(0, bucket.remaining_percent));
    const isLow = pct < 20;
    let paceHtml = "";
    if (withPace && showPace && typeof calculateQuotaPace === "function") {
      const pace = calculateQuotaPace("Weekly", bucket, tier);
      if (pace && typeof pace.expectedRemaining === "number") {
        const leftPct = Math.min(100, Math.max(0, pace.expectedRemaining)).toFixed(1);
        paceHtml = `<span class="mp-q-pace${pace.isOver ? " is-over" : ""}" style="left:${leftPct}%;"></span>`;
      }
    }
    return `
      <div class="mp-q-metric${isLow ? " is-low" : ""}">
        <div class="mp-q-metric-head">
          <span class="mp-q-label">${escapeHtml(label)}</span>
          <span class="mp-q-pct">${Math.round(pct)}%</span>
        </div>
        <div class="mp-q-track">
          <span class="mp-q-bar" style="width:${pct.toFixed(1)}%;"></span>
          ${paceHtml}
        </div>
      </div>
    `;
  };

  const renderPool = (poolKey, title, bucket5h, bucketWeekly) => {
    if (!bucket5h && !bucketWeekly) return "";
    const resetShort = !isFree && bucket5h ? compactQuotaResetLabel(bucket5h.reset_friendly) : "";
    return `
      <div class="mp-q-pool mp-q-pool-${poolKey}" data-quota-pool="${poolKey}">
        <div class="mp-q-head">
          <span class="mp-q-dot"></span>
          <span class="mp-q-name">${title}</span>
          ${resetShort ? `<span class="mp-q-reset">${escapeHtml(resetShort)}</span>` : ""}
        </div>
        ${!isFree ? renderBarRow("5h", bucket5h, false) : ""}
        ${renderBarRow(tr("周"), bucketWeekly, true)}
      </div>
    `;
  };

  return `
    ${renderPool("gemini", "Gemini", acc.gemini_5h, acc.gemini_weekly)}
    ${renderPool("claude", "Claude", acc.claude_5h, acc.claude_weekly)}
  `;
}

function openWebModelPicker(anchorEl, opts = {}) {
  if (typeof dismissWebWorkflowPicker === "function") dismissWebWorkflowPicker();
  const overlay = document.getElementById("model-picker-overlay");
  if (!overlay || !anchorEl) return;
  const families = buildModelFamilies();
  const rows = [];
  let prevProv = null;
  for (const fam of families) {
    const isGroupStart = prevProv !== null && prevProv !== fam.provider;
    if (!fam.hasEffortVariants && fam.variantsTopToBottom.length > 1 && fam.provider !== "orchestrator") {
      fam.variantsTopToBottom.forEach((v, idx) => {
        rows.push({
          id: v.id,
          title: v.rawName,
          compactTitle: null,
          provider: fam.provider,
          isGroupStart: idx === 0 ? isGroupStart : false,
          variants: [v],
          defaultVariantIndex: 0,
          hasSubmenu: false,
          hasInlineEffort: false,
          usesEffortLabels: false
        });
      });
    } else {
      let fullTitle = fam.name;
      if (fam.provider === "orchestrator") {
        const source = fam.name || fam.variantsTopToBottom[0]?.rawName || "Orchestrator";
        fullTitle = formatOrchestratorSlidePickerTitle(source);
      }
      rows.push({
        id: fam.id,
        title: fullTitle,
        compactTitle: null,
        provider: fam.provider,
        isGroupStart,
        variants: fam.variantsTopToBottom,
        defaultVariantIndex: fam.defaultVariantIndex,
        hasSubmenu: fam.variantsTopToBottom.length > 1,
        hasInlineEffort: fam.variantsTopToBottom.length > 1,
        usesEffortLabels: fam.hasEffortVariants || fam.hasAnyEffortVariants
      });
    }
    prevProv = fam.provider;
  }
  if (!rows.length) return;

  const anchor = anchorEl.getBoundingClientRect();
  const dragMode = Boolean(opts.dragMode);
  webPickerState = {
    rows,
    anchor,
    hoveredIdx: null,
    hoveredVariantIdx: null,
    dragMode,
    isTapMode: !dragMode,
    hasEnteredMenu: false,
    lastPointer: (typeof opts.pointerX === "number" && typeof opts.pointerY === "number")
      ? { x: opts.pointerX, y: opts.pointerY }
      : null,
    isHorizontalRowLocked: false
  };
  mountWebModelPicker();
  overlay.classList.remove("hidden");
  updateWebModelPickerHighlight();
}

function mountWebModelPicker() {
  const overlay = document.getElementById("model-picker-overlay");
  if (!overlay || !webPickerState) return;
  const { rows, anchor } = webPickerState;
  const winW = window.innerWidth;
  const rightW = winW < 380 ? 104 : (winW >= 420 ? 124 : 116);
  const colGap = 8;
  const hMargin = 12;
  const maxRoom = Math.max(184, winW - rightW - colGap - hMargin * 2);
  const leftW = Math.min(248, Math.max(Math.min(212, maxRoom), maxRoom));
  const leftX = winW < 520
    ? hMargin
    : Math.max(hMargin, Math.min(anchor.left, winW - leftW - rightW - colGap - hMargin));
  const bottomY = Math.max(16, window.innerHeight - anchor.top + 8);
  const maxHeight = Math.max(160, anchor.top - 24);
  const rightX = Math.min(leftX + leftW + colGap, winW - rightW - hMargin);

  const data = typeof currentCockpitQuotas !== "undefined" ? currentCockpitQuotas : null;
  const acc = data && (data.current_account || (data.accounts && data.accounts[0]));
  const quotaHtml = buildQuotaCompanionHtml(acc);
  const isActiveTurnRunning = typeof isCascadeRunning === "function" && activeCascadeId && isCascadeRunning(activeCascadeId);

  const leftRowsHtml = rows.map((row, idx) => {
    const activeVIdx = row.variants.findIndex((v) => v.id === activeModel);
    const isActive = activeVIdx >= 0;
    const provSafe = String(row.provider || "").replace(/[^a-zA-Z0-9_-]/g, "");
    const count = row.variants.length;
    const idleDotsHtml = row.hasInlineEffort
      ? `<span class="mp-idle-dots" aria-hidden="true">${'<span class="mp-idle-dot"></span>'.repeat(Math.min(count, 4))}</span>`
      : "";
    const nodeRailHtml = row.hasInlineEffort
      ? `<span class="mp-node-rail" aria-hidden="true">${row.variants.map((_, sIdx) => `<span class="mp-node-slot"><span class="mp-node-dot" data-stop-idx="${sIdx}"></span></span>`).join("")}</span>`
      : "";
    const classes = [
      "mp-row",
      `mp-prov-${provSafe}`,
      isActive ? "is-active" : "",
      row.isGroupStart ? "mp-group-start" : "",
      row.hasInlineEffort ? "has-effort" : "is-single"
    ].filter(Boolean).join(" ");

    return `
      <button type="button" class="${classes}" data-row-idx="${idx}">
        <span class="mp-fill" aria-hidden="true">
          ${COSMIC_STARS_SVG}
          <span class="mp-fill-cap"></span>
        </span>
        <span class="mp-row-fg">
          <span class="mp-check">${isActive ? "✓" : ""}</span>
          <span class="mp-title">${escapeHtml(row.title)}</span>
          <span class="mp-effort-badge"></span>
          ${idleDotsHtml}
        </span>
        ${nodeRailHtml}
      </button>
    `;
  }).join("");

  const rightCardHtml = quotaHtml
    ? `<div class="mp-card mp-right mp-quota-card" style="left:${rightX}px; bottom:${bottomY}px; width:${rightW}px;">${quotaHtml}</div>`
    : "";

  const runningBadgeHtml = isActiveTurnRunning
    ? `<div class="mp-running-pill">运行中 · 切换下轮生效</div>`
    : "";

  overlay.innerHTML = `
    <div class="mp-backdrop"></div>
    <div class="mp-top-hud" style="left:${leftX}px; width:${leftW}px; bottom:${bottomY + 8}px;">
      ${runningBadgeHtml}
      <div class="mp-live-pill hidden"></div>
    </div>
    <div class="mp-card mp-left" style="left:${leftX}px; bottom:${bottomY}px; width:${leftW}px; max-height:${maxHeight}px;">
      ${leftRowsHtml}
    </div>
    ${rightCardHtml}
  `;

  // Sync top HUD bottom offset to actual rendered left card height
  const leftCardEl = overlay.querySelector(".mp-left");
  const hudEl = overlay.querySelector(".mp-top-hud");
  if (leftCardEl && hudEl) {
    const rect = leftCardEl.getBoundingClientRect();
    hudEl.style.bottom = `${Math.max(20, window.innerHeight - rect.top + 8)}px`;
  }

  overlay.querySelector(".mp-backdrop")?.addEventListener("click", dismissWebModelPicker);

  overlay.querySelectorAll(".mp-left .mp-row").forEach((btn) => {
    const idx = parseInt(btn.getAttribute("data-row-idx"), 10);
    const row = rows[idx];

    btn.addEventListener("pointermove", (e) => {
      if (!webPickerState || webPickerState.dragMode || e.pointerType === "touch") return;
      const rect = btn.getBoundingClientRect();
      const vIdx = row.hasInlineEffort
        ? resolveVariantIndexForX(row, e.clientX - rect.left, rect.width, webPickerState.hoveredIdx === idx ? webPickerState.hoveredVariantIdx : null, 6)
        : 0;
      if (webPickerState.hoveredIdx !== idx || webPickerState.hoveredVariantIdx !== vIdx) {
        webPickerState.hoveredIdx = idx;
        webPickerState.hoveredVariantIdx = vIdx;
        updateWebModelPickerHighlight();
      }
    });

    btn.addEventListener("click", (e) => {
      e.stopPropagation();
      if (!webPickerState) return;
      if (!row.hasInlineEffort && row.variants[0]) {
        selectModel(row.variants[0].id);
        dismissWebModelPicker();
        return;
      }
      const rect = btn.getBoundingClientRect();
      const isTouchClick = e.pointerType === "touch" || (e.detail === 0);
      if (isTouchClick && webPickerState.hoveredIdx !== idx) {
        // First tap on unfocused multi-tier row on touch devices: focus row at active/default tier
        const activeVIdx = row.variants.findIndex((v) => v.id === activeModel);
        webPickerState.hoveredIdx = idx;
        webPickerState.hoveredVariantIdx = activeVIdx >= 0 ? activeVIdx : row.defaultVariantIndex;
        updateWebModelPickerHighlight();
        return;
      }
      const chosenIdx = resolveVariantIndexForX(row, e.clientX - rect.left, rect.width, null, 0);
      const chosen = row.variants[chosenIdx] || row.variants[row.defaultVariantIndex] || row.variants[0];
      if (chosen) selectModel(chosen.id);
      dismissWebModelPicker();
    });
  });

  leftCardEl?.addEventListener("mouseleave", () => {
    if (!webPickerState || webPickerState.dragMode) return;
    if (webPickerState.hoveredIdx !== null) {
      webPickerState.hoveredIdx = null;
      webPickerState.hoveredVariantIdx = null;
      updateWebModelPickerHighlight();
    }
  });
}

function updateWebModelPickerHighlight() {
  const overlay = document.getElementById("model-picker-overlay");
  if (!overlay || !webPickerState) return;
  const { rows, hoveredIdx, hoveredVariantIdx, dragMode } = webPickerState;

  overlay.querySelectorAll(".mp-left .mp-row").forEach((btn) => {
    const idx = parseInt(btn.getAttribute("data-row-idx"), 10);
    const row = rows[idx];
    if (!row) return;
    const isHovered = hoveredIdx === idx;
    const activeVIdx = row.variants.findIndex((v) => v.id === activeModel);
    const isActive = activeVIdx >= 0;

    let displayVIdx = null;
    if (isHovered) {
      displayVIdx = Math.min(Math.max(0, hoveredVariantIdx ?? row.defaultVariantIndex), row.variants.length - 1);
    } else if (isActive) {
      displayVIdx = activeVIdx;
    }

    const variant = displayVIdx !== null ? row.variants[displayVIdx] : null;
    const rank = variant ? variantEffortRank(variant) : -1;
    const tierKey = effortRankKey(rank);
    const count = row.variants.length;
    const fillPct = (isHovered || isActive)
      ? (row.hasInlineEffort ? effortFillPercent(displayVIdx ?? 0, count) : 100)
      : 0;

    btn.classList.toggle("is-hovered", isHovered);
    btn.classList.toggle("is-active", isActive);
    btn.classList.remove("tier-low", "tier-med", "tier-high", "tier-xhigh", "is-full-fill", "is-resting-xhigh");
    if (isHovered || isActive) {
      btn.classList.add(`tier-${tierKey}`);
    }
    if (fillPct >= 99) {
      btn.classList.add("is-full-fill");
    }
    if (!isHovered && isActive && rank === 3) {
      btn.classList.add("is-resting-xhigh");
    }

    const fillEl = btn.querySelector(".mp-fill");
    if (fillEl) {
      fillEl.style.width = `${fillPct}%`;
    }

    const badgeEl = btn.querySelector(".mp-effort-badge");
    if (badgeEl) {
      if (row.hasInlineEffort && variant) {
        badgeEl.textContent = row.usesEffortLabels ? effortShortLabel(variant.label, variant.id) : variant.label;
        badgeEl.classList.add("is-visible");
      } else {
        badgeEl.textContent = "";
        badgeEl.classList.remove("is-visible");
      }
    }

    if (row.hasInlineEffort) {
      btn.querySelectorAll(".mp-node-dot").forEach((dot) => {
        const sIdx = parseInt(dot.getAttribute("data-stop-idx"), 10);
        dot.classList.toggle("is-current", isHovered && sIdx === displayVIdx);
        dot.classList.toggle("is-reached", isHovered && displayVIdx !== null && sIdx <= displayVIdx);
      });
    }
  });

  // Update top live preview pill during touch drag
  const livePill = overlay.querySelector(".mp-live-pill");
  if (livePill) {
    if (dragMode && hoveredIdx !== null && rows[hoveredIdx]) {
      const row = rows[hoveredIdx];
      const vIdx = Math.min(Math.max(0, hoveredVariantIdx ?? row.defaultVariantIndex), row.variants.length - 1);
      const variant = row.variants[vIdx];
      const rank = variant ? variantEffortRank(variant) : -1;
      const tierKey = effortRankKey(rank);
      const provSafe = String(row.provider || "").replace(/[^a-zA-Z0-9_-]/g, "");
      const effortText = row.hasInlineEffort && variant
        ? (row.usesEffortLabels ? effortShortLabel(variant.label, variant.id) : variant.label)
        : "";
      livePill.className = `mp-live-pill mp-prov-${provSafe} tier-${tierKey}`;
      livePill.innerHTML = `${rank === 3 ? COSMIC_STARS_SVG : ""}<span class="mp-live-pill-fg"><span>${escapeHtml(row.title)}</span>${effortText ? `<span class="mp-live-sep">·</span><strong>${escapeHtml(effortText)}</strong>` : ""}</span>`;
    } else {
      livePill.className = "mp-live-pill hidden";
      livePill.innerHTML = "";
    }
  }

  // Sync right companion quota card focus highlight
  let highlightedPool = null;
  if (hoveredIdx !== null && rows[hoveredIdx]) {
    highlightedPool = modelPickerQuotaPool(rows[hoveredIdx]);
  } else {
    const activeRow = rows.find((r) => r.variants.some((v) => v.id === activeModel));
    if (activeRow) highlightedPool = modelPickerQuotaPool(activeRow);
  }
  const hasHoverHighlight = hoveredIdx !== null && highlightedPool !== null;
  overlay.querySelectorAll(".mp-quota-card .mp-q-pool").forEach((poolEl) => {
    const key = poolEl.getAttribute("data-quota-pool");
    poolEl.classList.toggle("is-focused", key === highlightedPool);
    poolEl.classList.toggle("is-dimmed", hasHoverHighlight && key !== highlightedPool);
  });
}

function updateWebModelPickerDrag(clientX, clientY) {
  const overlay = document.getElementById("model-picker-overlay");
  if (!overlay || !webPickerState || !webPickerState.dragMode) return;
  const leftCard = overlay.querySelector(".mp-left");
  if (!leftCard) return;
  const leftRect = leftCard.getBoundingClientRect();
  const anchor = webPickerState.anchor;
  const prev = webPickerState.lastPointer;
  webPickerState.lastPointer = { x: clientX, y: clientY };

  const onAnchor = clientX >= anchor.left - 16 && clientX <= anchor.right + 16 && clientY >= anchor.top - 12 && clientY <= anchor.bottom + 12;
  if (onAnchor || clientY > leftRect.bottom + 18 || clientY < leftRect.top - 52 || clientX < leftRect.left - 48) {
    webPickerState.isHorizontalRowLocked = false;
    if (webPickerState.hoveredIdx !== null || webPickerState.hoveredVariantIdx !== null) {
      webPickerState.hoveredIdx = null;
      webPickerState.hoveredVariantIdx = null;
      updateWebModelPickerHighlight();
    }
    return;
  }

  webPickerState.hasEnteredMenu = true;
  const rowEls = Array.from(overlay.querySelectorAll(".mp-left .mp-row"));
  if (!rowEls.length) return;

  let resolvedIdx = webPickerState.hoveredIdx;
  if (resolvedIdx !== null && rowEls[resolvedIdx] && webPickerState.rows[resolvedIdx]?.hasInlineEffort) {
    if (prev) {
      const dx = Math.abs(clientX - prev.x);
      const dy = Math.abs(clientY - prev.y);
      if (dx > dy * 1.35 && dx > 2) webPickerState.isHorizontalRowLocked = true;
    }
    const curRect = rowEls[resolvedIdx].getBoundingClientRect();
    const centerY = (curRect.top + curRect.bottom) / 2;
    const tol = webPickerState.isHorizontalRowLocked ? (curRect.height * 0.5 + 15) : (curRect.height * 0.5 + 1.5);
    if (Math.abs(clientY - centerY) > tol) {
      webPickerState.isHorizontalRowLocked = false;
      resolvedIdx = null;
    }
  } else {
    webPickerState.isHorizontalRowLocked = false;
    resolvedIdx = null;
  }

  if (resolvedIdx === null) {
    resolvedIdx = 0;
    let bestDist = Infinity;
    rowEls.forEach((el, idx) => {
      const r = el.getBoundingClientRect();
      if (clientY >= r.top && clientY <= r.bottom) {
        resolvedIdx = idx;
        bestDist = -1;
      } else if (bestDist >= 0) {
        const d = Math.abs(clientY - (r.top + r.bottom) / 2);
        if (d < bestDist) {
          bestDist = d;
          resolvedIdx = idx;
        }
      }
    });
  }

  const row = webPickerState.rows[resolvedIdx];
  const rowRect = rowEls[resolvedIdx].getBoundingClientRect();
  const sameRow = webPickerState.hoveredIdx === resolvedIdx;
  const nextVIdx = row.hasInlineEffort
    ? resolveVariantIndexForX(row, clientX - rowRect.left, rowRect.width, sameRow ? webPickerState.hoveredVariantIdx : null, 6)
    : 0;

  const changed = webPickerState.hoveredIdx !== resolvedIdx || webPickerState.hoveredVariantIdx !== nextVIdx;
  webPickerState.hoveredIdx = resolvedIdx;
  webPickerState.hoveredVariantIdx = nextVIdx;
  if (changed) {
    const rank = variantEffortRank(row.variants[nextVIdx]);
    if (typeof triggerHaptic === "function") triggerHaptic(rank === 3 ? "medium" : "light");
    updateWebModelPickerHighlight();
  }
}

function endWebModelPickerDrag() {
  if (!webPickerState) return;
  const { rows, hoveredIdx, hoveredVariantIdx, hasEnteredMenu } = webPickerState;
  webPickerState.dragMode = false;
  webPickerState.lastPointer = null;
  webPickerState.isHorizontalRowLocked = false;

  if (hoveredIdx !== null && rows[hoveredIdx]) {
    const row = rows[hoveredIdx];
    const vIdx = hoveredVariantIdx ?? row.defaultVariantIndex;
    const chosen = row.variants[vIdx] || row.variants[row.defaultVariantIndex] || row.variants[0];
    if (chosen) selectModel(chosen.id);
    dismissWebModelPicker();
    return;
  }

  if (!hasEnteredMenu) {
    webPickerState.isTapMode = true;
    updateWebModelPickerHighlight();
  } else {
    dismissWebModelPicker();
  }
}
