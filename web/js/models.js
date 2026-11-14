// 动态模型目录与默认模型。
//
// 网关 GET /gateway/models 返回 language_server 当前可用的模型（含内部枚举）和网关建议的各厂商默认模型；
// 输入框上方的「Gemini / Claude」胶囊切到设置里为对应厂商选定的具体模型。
// 取不到实时目录时退回内置的两个模型，行为与旧版一致。

const MODEL_FALLBACK = { gemini: "gemini-3.8-flash-high", claude: "claude-opus-4-6-thinking" };
const MODEL_ENUM_FALLBACK = { gemini: "MODEL_PLACEHOLDER_M318", claude: "MODEL_PLACEHOLDER_M26" };
const MODEL_CATALOG_KEY = "agy_model_catalog";
const MODEL_DEFAULT_KEY = (provider) => `agy_default_${provider}_model`;
const IGNORED_PICKER_TAGS = new Set(["new", "fast", "notice", "提示"]);
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
  "gpt-oss-120b-medium": "GPT-OSS 120B (Medium)",
};

let modelCatalog = { models: [], defaults: {}, live: false };

const CLAUDE_ENUM_TOKENS = ["m26", "m35", "m400", "m401", "m402", "m403", "m404", "m405"];

function isClaudeModel(id) {
  const lower = String(id || "").toLowerCase();
  if (lower.includes("claude")) return true;
  return CLAUDE_ENUM_TOKENS.some((tok) => lower.includes(tok));
}

// 输入框上方胶囊的文字：只有厂商名
function modelProviderLabel(id) {
  if (isClaudeModel(id)) return "Claude";
  if (String(id || "").toLowerCase().includes("gpt")) return "GPT";
  return "Gemini";
}

function formatModelIDAsDisplayName(id) {
  const trimmed = String(id || "").trim();
  if (!trimmed) return "";
  const staticHit = MODEL_STATIC_NAMES[trimmed.toLowerCase()];
  if (staticHit) return staticHit;
  const tokens = trimmed.split("-").filter(Boolean);
  if (!tokens.length) return trimmed;
  const qualifiers = new Set(["high", "medium", "low", "thinking"]);
  let qualifier = null;
  if (tokens.length > 1 && qualifiers.has(tokens[tokens.length - 1].toLowerCase())) {
    const q = tokens.pop().toLowerCase();
    qualifier = q.charAt(0).toUpperCase() + q.slice(1);
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
  const cleanTag = String(tag || "").trim();
  if (cleanTag && !IGNORED_PICKER_TAGS.has(cleanTag.toLowerCase())) {
    return `${name} · ${cleanTag}`;
  }
  return name;
}

function modelDisplayName(id) {
  const m = modelCatalog.models.find((x) => x.id === id);
  return (m && m.name) ? m.name : formatModelIDAsDisplayName(id);
}

// 默认模型：用户在设置里选的（且仍在实时目录里）→ 网关建议 → 内置兜底
function getDefaultModel(provider) {
  let saved = "";
  try {
    saved = localStorage.getItem(MODEL_DEFAULT_KEY(provider)) || "";
  } catch (_) {}
  const inCatalog = (id) => modelCatalog.models.some((m) => m.id === id && m.provider === provider);
  if (saved && (!modelCatalog.live || inCatalog(saved))) return saved;
  return modelCatalog.defaults[provider] || modelCatalog.models.find((m) => m.provider === provider)?.id || MODEL_FALLBACK[provider];
}

function setDefaultModel(provider, id) {
  try {
    localStorage.setItem(MODEL_DEFAULT_KEY(provider), id);
  } catch (_) {}
}

// 胶囊点击后要切到的模型：Claude → Gemini 默认；其它 → Claude 默认
function modelToggleTarget(current) {
  return isClaudeModel(current) ? getDefaultModel("gemini") : getDefaultModel("claude");
}

// language_server 里切换默认模型用的内部枚举
function modelEnumFor(id) {
  const m = modelCatalog.models.find((x) => x.id === id);
  if (m && m.enum) return m.enum;
  return isClaudeModel(id) ? MODEL_ENUM_FALLBACK.claude : MODEL_ENUM_FALLBACK.gemini;
}

// 网关推送的 activeModel 如何落到界面状态：已是可读模型 id 则原样保留；
// 只拿到未解析的裸枚举（MODEL_...）时退回该厂商的默认模型。
function resolveActiveModel(raw, current) {
  const value = String(raw || "").trim();
  if (!value) return current;
  if (value.toUpperCase().startsWith("MODEL_")) {
    const m = modelCatalog.models.find((x) => x.enum === value);
    if (m) return m.id;
    return isClaudeModel(value) ? getDefaultModel("claude") : getDefaultModel("gemini");
  }
  return value;
}

function applyModelCatalog(data) {
  if (!data || !Array.isArray(data.models) || data.models.length === 0) return;
  modelCatalog = { models: data.models, defaults: data.defaults || {}, live: data.live === true };
  if (typeof updateModelSwitchUI === "function") updateModelSwitchUI();
  renderDefaultModelSettings();
}

async function loadModelCatalog(force = false) {
  // 先用上次缓存，保证离线 / 网关慢时设置页也有内容
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

// 设置页「默认模型」两个下拉框
function renderDefaultModelSettings() {
  for (const provider of ["gemini", "claude"]) {
    const select = document.getElementById(`settings-default-${provider}`);
    if (!select) continue;
    const options = modelCatalog.models.filter((m) => m.provider === provider);
    const current = getDefaultModel(provider);
    if (modelCatalog.live && current) setDefaultModel(provider, current);
    const list = options.length ? options : [{ id: current, name: modelDisplayName(current) }];
    select.innerHTML = list.map((m) => {
      const label = formatPickerLabel(m.name || modelDisplayName(m.id), m.tag);
      return `<option value="${escapeHtml(m.id)}">${escapeHtml(label)}</option>`;
    }).join("");
    select.value = current;
  }
  const note = document.getElementById("settings-models-note");
  if (note) {
    note.textContent = modelCatalog.live ? "" : "网关暂时取不到实时模型列表，当前显示内置列表。";
    note.classList.toggle("hidden", modelCatalog.live);
  }
}

function initModelSettings() {
  for (const provider of ["gemini", "claude"]) {
    document.getElementById(`settings-default-${provider}`)?.addEventListener("change", (e) => {
      setDefaultModel(provider, e.target.value);
      // 当前正用着该厂商的模型时，立刻对齐到新的默认
      if (typeof activeModel !== "undefined" && (provider === "claude") === isClaudeModel(activeModel) && activeModel !== e.target.value) {
        activeModel = e.target.value;
        try {
          localStorage.setItem("agy_active_model", activeModel);
        } catch (_) {}
        if (typeof updateModelSwitchUI === "function") updateModelSwitchUI();
      }
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
  renderDefaultModelSettings();
  loadModelCatalog();
}
