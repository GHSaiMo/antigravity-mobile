// 动态模型目录与默认模型。
//
// 网关 GET /gateway/models 返回 language_server 当前可用的模型（含内部枚举）和网关建议的各厂商默认模型；
// 输入框上方的「Gemini / Claude」胶囊切到设置里为对应厂商选定的具体模型。
// 取不到实时目录时退回内置的两个模型，行为与旧版一致。

const MODEL_FALLBACK = { gemini: "gemini-3.8-flash-high", claude: "claude-opus-4-6-thinking" };
const MODEL_ENUM_FALLBACK = { gemini: "MODEL_PLACEHOLDER_M318", claude: "MODEL_PLACEHOLDER_M26" };
const MODEL_CATALOG_KEY = "agy_model_catalog";
const MODEL_DEFAULT_KEY = (provider) => `agy_default_${provider}_model`;

let modelCatalog = { models: [], defaults: {}, live: false };

function isClaudeModel(id) {
  const lower = String(id || "").toLowerCase();
  return lower.includes("claude") || lower.includes("m26");
}

// 输入框上方胶囊的文字：只有厂商名
function modelProviderLabel(id) {
  if (isClaudeModel(id)) return "Claude";
  if (String(id || "").toLowerCase().includes("gpt")) return "GPT";
  return "Gemini";
}

function modelDisplayName(id) {
  const m = modelCatalog.models.find((x) => x.id === id);
  return m ? m.name : id;
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

async function loadModelCatalog() {
  // 先用上次缓存，保证离线 / 网关慢时设置页也有内容
  try {
    const cached = JSON.parse(localStorage.getItem(MODEL_CATALOG_KEY) || "null");
    if (cached) applyModelCatalog(cached);
  } catch (_) {}
  try {
    const resp = await fetch("/gateway/models");
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
    const list = options.length ? options : [{ id: current, name: current }];
    select.innerHTML = list.map((m) => `<option value="${escapeHtml(m.id)}">${escapeHtml(m.name || m.id)}</option>`).join("");
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
  renderDefaultModelSettings();
  loadModelCatalog();
}
