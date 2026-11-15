// 动态模型目录与常用模型（常用模型 1 / 常用模型 2 + 长按全模型二级选单）。
//
// 网关 GET /gateway/models 返回 language_server 当前可用的模型（含内部枚举）和建议默认模型；
// 设置页配置两个常用模型（均支持全部模型与全部思考强度），新会话首选常用模型 1；
// 单击模型胶囊在两个常用模型间快速切换；长按/右键列出全部模型（Gemini 蓝 / Claude 橙 / Orchestrator 钛灰）及二级思考强度（Low -> Medium -> High -> Extra，默认 Medium）。

const MODEL_FALLBACK = {
  gemini: "gemini-3.8-flash-high",
  claude: "claude-opus-4-6-thinking",
  orchestrator: "gemini-pro-orchestrator"
};
const MODEL_ENUM_FALLBACK = {
  gemini: "MODEL_PLACEHOLDER_M318",
  claude: "MODEL_PLACEHOLDER_M26",
  orchestrator: "MODEL_PLACEHOLDER_M667"
};
const MODEL_CATALOG_KEY = "agy_model_catalog";
const MODEL_DEFAULT_KEY = (provider) => `agy_default_${provider}_model`;
const FAVORITE_MODEL_KEY = (slot) => `agy_favorite_model_${slot}`;
const IGNORED_PICKER_TAGS = new Set(["new", "fast", "notice", "提示"]);
const CLAUDE_ENUM_TOKENS = ["m26", "m35", "m400", "m401", "m402", "m403", "m404", "m405"];
const ORCHESTRATOR_ENUM_TOKENS = ["m667", "m681"];
const MODEL_FALLBACK_ORDER = [
  "gemini-4-carbon-medium", "gemini-4-carbon-high", "gemini-4-carbon-low",
  "gemini-4-argon-medium", "gemini-4-argon-high", "gemini-4-argon-low",
  "gemini-4-hydrogen-xhigh", "gemini-4-hydrogen-high", "gemini-4-hydrogen-medium", "gemini-4-hydrogen-low",
  "gemini-3.8-flash-high", "gemini-3.8-flash-medium", "gemini-3.8-flash-low",
  "claude-opus-5-5-high", "claude-opus-5-5-medium", "claude-opus-5-5-low",
  "claude-sonnet-5-5-high", "claude-sonnet-5-5-medium", "claude-sonnet-5-5-low",
  "claude-opus-4-6-thinking", "claude-sonnet-4-6",
  "gemini-pro-orchestrator", "gemini-pro-orchestrator-argon"
];
const MODEL_STATIC_NAMES = {
  "gemini-4-hydrogen-xhigh": "Gemini 4 Hydrogen (Extra)",
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
  "gemini-pro-orchestrator": "Orchestrator (Carbon)",
  "gemini-pro-orchestrator-argon": "Orchestrator (Argon)",
  "orchestrator-carbon": "Orchestrator (Carbon)",
  "orchestrator-argon": "Orchestrator (Argon)",
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
  return String(raw || "").trim().replace(/\s*\(Xhigh\)/i, " (Extra)");
}

function formatModelIDAsDisplayName(id) {
  const trimmed = String(id || "").trim();
  if (!trimmed) return "";
  const staticHit = MODEL_STATIC_NAMES[trimmed.toLowerCase()];
  if (staticHit) return staticHit;
  const tokens = trimmed.split("-").filter(Boolean);
  if (!tokens.length) return trimmed;
  const qualifiers = new Set(["xhigh", "extra", "high", "medium", "low", "thinking"]);
  let qualifier = null;
  if (tokens.length > 1 && qualifiers.has(tokens[tokens.length - 1].toLowerCase())) {
    const q = tokens.pop().toLowerCase();
    qualifier = q === "xhigh" ? "Extra" : (q.charAt(0).toUpperCase() + q.slice(1));
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

function compactModelDisplayName(id) {
  const full = modelDisplayName(id);
  const match = full.match(/^(.+?)\s*\(([^)]+)\)$/);
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
    else if (vl === "xhigh") variant = "Extra";
    return `${base} · ${variant}`;
  }
  let base = full;
  const bl = base.toLowerCase();
  if (bl.startsWith("gemini 4 ")) base = base.slice("gemini 4 ".length);
  else if (bl.startsWith("gemini ")) base = base.slice("gemini ".length);
  else if (bl.startsWith("claude ")) base = base.slice("claude ".length);
  else if (bl.startsWith("orchestrator")) base = "Orch";
  return base;
}

function providerSortRank(p) {
  switch (String(p || "").toLowerCase()) {
    case "gemini": return 0;
    case "claude": return 1;
    case "orchestrator": return 2;
    default: return 3;
  }
}

function allSortedModelOptions() {
  const filtered = (modelCatalog.models || []).filter((m) => !String(m.id || "").toLowerCase().includes("gpt-oss"));
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
  const s = String(labelOrId || "").toLowerCase();
  if (/\blow\b/.test(s) || s.endsWith("-low")) return 0;
  if (/\bmedium\b|\bmed\b/.test(s) || s.endsWith("-medium")) return 1;
  if (/\bhigh\b/.test(s) && !/\bxhigh\b|\bextra\b/.test(s) || s.endsWith("-high")) return 2;
  if (/\bxhigh\b|\bextra\b/.test(s) || s.endsWith("-xhigh")) return 3;
  return -1;
}

function effortDisplayLabel(label, id) {
  const r = parseEffortRank(label) >= 0 ? parseEffortRank(label) : parseEffortRank(id);
  return ["Low", "Medium", "High", "Extra"][r] || label;
}

function buildModelFamilies() {
  const opts = allSortedModelOptions();
  const map = new Map();
  const order = [];
  for (const opt of opts) {
    const fullName = opt.name ? normalizeDisplayName(opt.name) : modelDisplayName(opt.id);
    let familyName = fullName;
    let variantLabel = "默认";
    const m = fullName.match(/^(.+?)\s*\(([^)]+)\)$/);
    if (m) {
      familyName = m[1].trim();
      variantLabel = m[2].trim();
    }
    const kind = modelProviderKind(opt.id);
    const prov = (kind === "orchestrator" || !opt.provider) ? kind : opt.provider.toLowerCase();
    const variant = { id: opt.id, label: variantLabel, rawName: fullName, tag: opt.tag || null };
    if (!map.has(familyName)) {
      order.push(familyName);
      map.set(familyName, { id: familyName, name: familyName, provider: prov, variants: [variant] });
    } else {
      map.get(familyName).variants.push(variant);
    }
  }
  const families = order.map((k) => {
    const fam = map.get(k);
    const hasEffort = fam.variants.length > 1 && fam.variants.every((v) => parseEffortRank(v.label) >= 0 || parseEffortRank(v.id) >= 0);
    const ordered = hasEffort
      ? [...fam.variants].sort((a, b) => {
          const ra = parseEffortRank(a.label) >= 0 ? parseEffortRank(a.label) : parseEffortRank(a.id);
          const rb = parseEffortRank(b.label) >= 0 ? parseEffortRank(b.label) : parseEffortRank(b.id);
          return ra - rb; // Low -> Medium -> High -> Extra
        })
      : fam.variants;
    const medIdx = ordered.findIndex((v) => (parseEffortRank(v.label) >= 0 ? parseEffortRank(v.label) : parseEffortRank(v.id)) === 1);
    const highIdx = ordered.findIndex((v) => (parseEffortRank(v.label) >= 0 ? parseEffortRank(v.label) : parseEffortRank(v.id)) === 2);
    const defaultIdx = medIdx >= 0 ? medIdx : (highIdx >= 0 ? highIdx : 0);
    return {
      ...fam,
      hasEffortVariants: hasEffort,
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
  const selectable = (modelCatalog.models || []).filter((m) => !String(m.id || "").toLowerCase().includes("gpt-oss"));
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

// Web 端长按 / 右键全模型二级菜单（蓝色 Gemini / 橙色 Claude / 石墨钛灰 Orchestrator；二级菜单 Low -> Medium -> High -> Extra，默认 Medium）
let webPickerState = null;

function dismissWebModelPicker() {
  const overlay = document.getElementById("model-picker-overlay");
  if (overlay) {
    overlay.classList.add("hidden");
    overlay.innerHTML = "";
  }
  webPickerState = null;
}

function openWebModelPicker(anchorEl) {
  if (typeof dismissWebWorkflowPicker === "function") dismissWebWorkflowPicker();
  const overlay = document.getElementById("model-picker-overlay");
  if (!overlay || !anchorEl) return;
  const families = buildModelFamilies();
  const rows = [];
  let prevProv = null;
  for (const fam of families) {
    const isGroupStart = prevProv !== null && prevProv !== fam.provider;
    if (!fam.hasEffortVariants && fam.variantsTopToBottom.length > 1) {
      fam.variantsTopToBottom.forEach((v, idx) => {
        rows.push({
          id: v.id,
          title: v.rawName,
          provider: fam.provider,
          isGroupStart: idx === 0 ? isGroupStart : false,
          variants: [v],
          defaultVariantIndex: 0,
          hasSubmenu: false,
          usesEffortLabels: false
        });
      });
    } else {
      rows.push({
        id: fam.id,
        title: fam.name,
        provider: fam.provider,
        isGroupStart,
        variants: fam.variantsTopToBottom,
        defaultVariantIndex: fam.defaultVariantIndex,
        hasSubmenu: fam.variantsTopToBottom.length > 1,
        usesEffortLabels: fam.hasEffortVariants
      });
    }
    prevProv = fam.provider;
  }
  if (!rows.length) return;

  const anchor = anchorEl.getBoundingClientRect();
  const activeIdx = rows.findIndex((r) => r.variants.some((v) => v.id === activeModel));
  const hoveredIdx = activeIdx >= 0 && rows[activeIdx].hasSubmenu ? activeIdx : null;
  webPickerState = { rows, anchor, hoveredIdx };
  renderWebModelPicker();
  overlay.classList.remove("hidden");
}

function renderWebModelPicker() {
  const overlay = document.getElementById("model-picker-overlay");
  if (!overlay || !webPickerState) return;
  const { rows, anchor, hoveredIdx } = webPickerState;
  const leftX = Math.max(12, Math.min(anchor.left, window.innerWidth - 336));
  const bottomY = Math.max(16, window.innerHeight - anchor.top + 8);
  const maxHeight = Math.max(160, anchor.top - 20);

  const leftRowsHtml = rows.map((row, idx) => {
    const isActive = row.variants.some((v) => v.id === activeModel);
    const isHovered = hoveredIdx === idx;
    const classes = [
      "mp-row",
      `mp-prov-${row.provider}`,
      isActive ? "is-active" : "",
      isHovered ? "is-hovered" : "",
      row.isGroupStart ? "mp-group-start" : ""
    ].filter(Boolean).join(" ");
    return `
      <button type="button" class="${classes}" data-row-idx="${idx}">
        <span class="mp-check">${isActive ? "✓" : ""}</span>
        <span class="mp-title">${escapeHtml(row.title)}</span>
        ${row.hasSubmenu ? '<span class="mp-arrow">›</span>' : ""}
      </button>
    `;
  }).join("");

  let rightCardHtml = "";
  if (hoveredIdx !== null && rows[hoveredIdx]?.hasSubmenu) {
    const row = rows[hoveredIdx];
    const rightVariantsHtml = row.variants.map((v, vIdx) => {
      const isActive = v.id === activeModel;
      const isDefault = vIdx === row.defaultVariantIndex;
      const label = row.usesEffortLabels ? effortDisplayLabel(v.label, v.id) : v.label;
      const classes = [
        "mp-row",
        `mp-prov-${row.provider}`,
        isActive ? "is-active" : "",
        isDefault ? "is-default" : ""
      ].filter(Boolean).join(" ");
      return `
        <button type="button" class="${classes}" data-variant-id="${escapeHtml(v.id)}">
          <span class="mp-title">${escapeHtml(label)}</span>
          <span class="mp-check">${isActive ? "✓" : ""}</span>
        </button>
      `;
    }).join("");
    const rightLeft = Math.min(leftX + 218, window.innerWidth - 118);
    rightCardHtml = `
      <div class="mp-card mp-right mp-card-prov-${row.provider}" style="left:${rightLeft}px; bottom:${bottomY}px;">
        ${rightVariantsHtml}
      </div>
    `;
  }

  overlay.innerHTML = `
    <div class="mp-backdrop"></div>
    <div class="mp-card mp-left" style="left:${leftX}px; bottom:${bottomY}px; max-height:${maxHeight}px;">
      ${leftRowsHtml}
    </div>
    ${rightCardHtml}
  `;

  overlay.querySelector(".mp-backdrop")?.addEventListener("click", dismissWebModelPicker);
  overlay.querySelectorAll(".mp-left .mp-row").forEach((btn) => {
    const idx = parseInt(btn.getAttribute("data-row-idx"), 10);
    const row = rows[idx];
    btn.addEventListener("mouseenter", () => {
      if (row.hasSubmenu && webPickerState && webPickerState.hoveredIdx !== idx) {
        webPickerState.hoveredIdx = idx;
        renderWebModelPicker();
      }
    });
    btn.addEventListener("click", (e) => {
      e.stopPropagation();
      if (!row.hasSubmenu && row.variants[0]) {
        selectModel(row.variants[0].id);
        dismissWebModelPicker();
      } else if (webPickerState.hoveredIdx === idx) {
        const chosen = row.variants[row.defaultVariantIndex] || row.variants[0];
        if (chosen) selectModel(chosen.id);
        dismissWebModelPicker();
      } else {
        webPickerState.hoveredIdx = idx;
        renderWebModelPicker();
      }
    });
  });
  overlay.querySelectorAll(".mp-right .mp-row").forEach((btn) => {
    btn.addEventListener("click", (e) => {
      e.stopPropagation();
      const id = btn.getAttribute("data-variant-id");
      if (id) selectModel(id);
      dismissWebModelPicker();
    });
  });
}
