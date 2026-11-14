// Workflow 模板面板（Web 端对齐 iOS/Android WorkflowTemplateSheet，支持长按上滑快捷菜单、分区拖拽、增删改、左滑删除与网关同步）

const WORKFLOW_CACHE_KEY = "agy_workflow_templates_v1";
const WORKFLOW_GUIDE_DISMISSED_KEY = "agy_wf_quick_guide_dismissed_v1";

const DEFAULT_WORKFLOW_TEMPLATES = [
  {
    id: "preset-parallel-review",
    title: "并发代码审计",
    subtitle: "多维度安全 / 性能 / 类型检查",
    icon: "security",
    color: "purple",
    inQuickMenu: false,
    templatePrompt:
      "/workflow 请使用 run_workflow 动态编排工作流，以 parallel 并发对 [模块/文件路径] 进行多维度审计（安全/性能/类型规范），并在最终阶段输出结构化修复建议。"
  },
  {
    id: "preset-test-runner",
    title: "并发测试修复",
    subtitle: "并行运行单测并生成修复方案",
    icon: "bug",
    color: "orange",
    inQuickMenu: false,
    templatePrompt:
      "/workflow 请使用 run_workflow 编排自动化单测，以 parallel 同时运行 [测试目标或路径] 的单元测试，聚合失败信息并生成修复方案。"
  },
  {
    id: "preset-deep-research",
    title: "深度专题调研",
    subtitle: "分派子代理调研并输出报告",
    icon: "search",
    color: "blue",
    inQuickMenu: false,
    templatePrompt:
      "/workflow 请使用 run_workflow 编排调研工作流，分派子代理分别调查 [调研主题] 的核心技术方案、竞品对比与最佳实践，并汇总为结构化报告。"
  },
  {
    id: "preset-custom-workflow",
    title: "自定义多智能体任务",
    subtitle: "自由编排分阶段并行工作流",
    icon: "workflow",
    color: "indigo",
    inQuickMenu: false,
    templatePrompt:
      "/workflow 请使用 run_workflow 编写 Python 动态工作流脚本，分阶段执行以下任务：[描述具体任务]，要求使用 parallel 并发加速并给出 finish 契约结果。"
  }
];

// 与 iOS / Android 像素级对齐的统一 24×24 矢量图标路径
const WORKFLOW_ICON_CATALOG = [
  {
    key: "workflow",
    label: "工作流",
    path: "M 6 3.3 C 7.5 3.3 8.7 4.5 8.7 6 C 8.7 7.5 7.5 8.7 6 8.7 C 4.5 8.7 3.3 7.5 3.3 6 C 3.3 4.5 4.5 3.3 6 3.3 Z M 18 3.3 C 19.5 3.3 20.7 4.5 20.7 6 C 20.7 7.5 19.5 8.7 18 8.7 C 16.5 8.7 15.3 7.5 15.3 6 C 15.3 4.5 16.5 3.3 18 3.3 Z M 12 15.3 C 13.5 15.3 14.7 16.5 14.7 18 C 14.7 19.5 13.5 20.7 12 20.7 C 10.5 20.7 9.3 19.5 9.3 18 C 9.3 16.5 10.5 15.3 12 15.3 Z M 8.7 6 L 15.3 6 M 7.3 8.4 L 10.7 15.6 M 16.7 8.4 L 13.3 15.6"
  },
  {
    key: "security",
    label: "安全",
    path: "M 12 2.5 L 19.5 5.8 L 19.5 11.5 C 19.5 16.5 16.2 20.0 12 21.5 C 7.8 20.0 4.5 16.5 4.5 11.5 L 4.5 5.8 L 12 2.5 Z M 9 12 L 11.2 14.2 L 15.4 9.8"
  },
  {
    key: "bug",
    label: "Bug",
    path: "M 9 7.5 L 9 6.5 C 9 4.84 10.34 3.5 12 3.5 C 13.66 3.5 15 4.84 15 6.5 L 15 7.5 M 8.2 2.5 L 10 4.2 M 15.8 2.5 L 14 4.2 M 8.5 7.5 L 15.5 7.5 C 16.88 7.5 18 8.62 18 10 L 18 14 C 18 17.31 15.31 20 12 20 C 8.69 20 6 17.31 6 14 L 6 10 C 6 8.62 7.12 7.5 8.5 7.5 Z M 12 11 L 12 20 M 6 9.5 L 3.2 7.5 M 6 13.2 L 2.5 13.2 M 6.4 16.8 L 3.2 19.2 M 18 9.5 L 20.8 7.5 M 18 13.2 L 21.5 13.2 M 17.6 16.8 L 20.8 19.2"
  },
  {
    key: "search",
    label: "调研",
    path: "M 10.8 4 C 14.56 4 17.6 7.04 17.6 10.8 C 17.6 14.56 14.56 17.6 10.8 17.6 C 7.04 17.6 4 14.56 4 10.8 C 4 7.04 7.04 4 10.8 4 Z M 15.8 15.8 L 20.5 20.5"
  },
  {
    key: "bolt",
    label: "闪电",
    path: "M 13.2 2.5 L 4.5 13.5 L 11.5 13.5 L 10.2 21.5 L 19.5 10.5 L 12.5 10.5 L 13.2 2.5 Z"
  },
  {
    key: "code",
    label: "代码",
    path: "M 7.5 7.5 L 3 12 L 7.5 16.5 M 16.5 7.5 L 21 12 L 16.5 16.5 M 13.8 5 L 10.2 19"
  },
  {
    key: "terminal",
    label: "终端",
    path: "M 5.5 4 L 18.5 4 C 19.88 4 21 5.12 21 6.5 L 21 17.5 C 21 18.88 19.88 20 18.5 20 L 5.5 20 C 4.12 20 3 18.88 3 17.5 L 3 6.5 C 3 5.12 4.12 4 5.5 4 Z M 7 9.5 L 10.5 12.5 L 7 15.5 M 12.5 15.5 L 17 15.5"
  },
  {
    key: "doc",
    label: "文档",
    path: "M 14 2.5 L 6.5 2.5 C 5.4 2.5 4.5 3.4 4.5 4.5 L 4.5 19.5 C 4.5 20.6 5.4 21.5 6.5 21.5 L 17.5 21.5 C 18.6 21.5 19.5 20.6 19.5 19.5 L 19.5 8 L 14 2.5 Z M 14 2.5 L 14 8 L 19.5 8 M 8.5 13 L 15.5 13 M 8.5 17 L 13.5 17"
  },
  {
    key: "build",
    label: "编译",
    path: "M 9 3.2 C 12.0 2.7 14.8 3.7 16.8 5.7 L 21 9.9 L 18.2 12.7 L 14 8.5 C 12.5 7.0 10.8 6.4 9 6.7 L 9 3.2 Z M 13.5 8.1 L 3.4 18.2 C 2.6 19.0 2.6 20.3 3.4 21.1 C 4.2 21.9 5.5 21.9 6.3 21.1 L 16.4 11.0"
  },
  {
    key: "sparkles",
    label: "智能",
    path: "M 11 4 L 12.8 9.7 L 18.5 11.5 L 12.8 13.3 L 11 19 L 9.2 13.3 L 3.5 11.5 L 9.2 9.7 L 11 4 Z M 18.5 2.8 L 18.5 7.2 M 16.3 5 L 20.7 5"
  },
  {
    key: "git",
    label: "分支",
    path: "M 6 3.5 C 7.38 3.5 8.5 4.62 8.5 6 C 8.5 7.38 7.38 8.5 6 8.5 C 4.62 8.5 3.5 7.38 3.5 6 C 3.5 4.62 4.62 3.5 6 3.5 Z M 6 15.5 C 7.38 15.5 8.5 16.62 8.5 18 C 8.5 19.38 7.38 20.5 6 20.5 C 4.62 20.5 3.5 19.38 3.5 18 C 3.5 16.62 4.62 15.5 6 15.5 Z M 18 5.5 C 19.38 5.5 20.5 6.62 20.5 8 C 20.5 9.38 19.38 10.5 18 10.5 C 16.62 10.5 15.5 9.38 15.5 8 C 15.5 6.62 16.62 5.5 18 5.5 Z M 6 8.5 L 6 15.5 M 18 10.5 C 18 14.5 14.5 15.5 8.5 15.5"
  },
  {
    key: "merge",
    label: "合并",
    path: "M 6 3.5 C 7.38 3.5 8.5 4.62 8.5 6 C 8.5 7.38 7.38 8.5 6 8.5 C 4.62 8.5 3.5 7.38 3.5 6 C 3.5 4.62 4.62 3.5 6 3.5 Z M 6 15.5 C 7.38 15.5 8.5 16.62 8.5 18 C 8.5 19.38 7.38 20.5 6 20.5 C 4.62 20.5 3.5 19.38 3.5 18 C 3.5 16.62 4.62 15.5 6 15.5 Z M 18 15.5 C 19.38 15.5 20.5 16.62 20.5 18 C 20.5 19.38 19.38 20.5 18 20.5 C 16.62 20.5 15.5 19.38 15.5 18 C 15.5 16.62 16.62 15.5 18 15.5 Z M 6 8.5 L 6 15.5 M 6 8.5 C 6 13.5 11 18 15.5 18"
  },
  {
    key: "pipeline",
    label: "流水线",
    path: "M 4.5 11 C 5.0 6.8 8.6 3.8 12.8 4.0 C 15.8 4.1 18.4 5.8 19.6 8.5 M 20 4.5 L 20 8.8 L 15.7 8.8 M 19.5 13 C 19.0 17.2 15.4 20.2 11.2 20.0 C 8.2 19.9 5.6 18.2 4.4 15.5 M 4 19.5 L 4 15.2 L 8.3 15.2"
  },
  {
    key: "checklist",
    label: "清单",
    path: "M 3.5 7.5 L 5.8 9.8 L 9.8 5.5 M 13 7.8 L 20.5 7.8 M 3.5 16.5 L 5.8 18.8 L 9.8 14.5 M 13 16.8 L 20.5 16.8"
  },
  {
    key: "report",
    label: "报告",
    path: "M 3.5 4 L 3.5 20 L 20.5 20 M 8 16.5 L 8 12 M 12.8 16.5 L 12.8 8.5 M 17.6 16.5 L 17.6 5.5"
  }
];

const WORKFLOW_COLOR_CATALOG = [
  { key: "indigo", label: "靛蓝", hex: "#5e5ce6" },
  { key: "blue", label: "蓝色", hex: "#0a84ff" },
  { key: "purple", label: "紫色", hex: "#bf5af2" },
  { key: "green", label: "绿色", hex: "#30d158" },
  { key: "orange", label: "橙色", hex: "#ff9f0a" },
  { key: "red", label: "红色", hex: "#ff453a" },
  { key: "teal", label: "青色", hex: "#30b0c7" }
];

const workflowState = {
  templates: [],
  editing: false,
  formMode: null, // null | { mode: "add"|"edit", item: {...} }
  syncing: false,
  error: "",
  confirmReset: false
};

let webWorkflowPickerState = null;

function normalizeWorkflowIconKey(iconKey) {
  if (iconKey === "star") return "build";
  return WORKFLOW_ICON_CATALOG.some((ic) => ic.key === iconKey) ? iconKey : "workflow";
}

function normalizeWorkflowTemplate(raw) {
  const src = raw && typeof raw === "object" ? raw : {};
  return {
    id: String(src.id || "").trim() || generateWorkflowTemplateId(),
    title: String(src.title || "").trim(),
    subtitle: String(src.subtitle || "").trim(),
    icon: normalizeWorkflowIconKey(String(src.icon || "workflow").trim()),
    color: WORKFLOW_COLOR_CATALOG.some((c) => c.key === src.color) ? src.color : "indigo",
    templatePrompt: String(src.templatePrompt || "").trim(),
    inQuickMenu: Boolean(src.inQuickMenu)
  };
}

function partitionWorkflowTemplates(list) {
  const arr = Array.isArray(list) ? list.map(normalizeWorkflowTemplate) : [];
  const quick = arr.filter((t) => t.inQuickMenu);
  const more = arr.filter((t) => !t.inQuickMenu);
  return [...quick, ...more];
}

function workflowColorHex(colorKey) {
  const found = WORKFLOW_COLOR_CATALOG.find((c) => c.key === colorKey);
  return found ? found.hex : "#5e5ce6";
}

function workflowIconSvg(iconKey, size = 18) {
  const resolved = normalizeWorkflowIconKey(iconKey);
  const entry = WORKFLOW_ICON_CATALOG.find((ic) => ic.key === resolved) || WORKFLOW_ICON_CATALOG[0];
  return `<svg width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round"><path d="${entry.path}"></path></svg>`;
}

function isWorkflowGuideDismissed() {
  try {
    return localStorage.getItem(WORKFLOW_GUIDE_DISMISSED_KEY) === "1";
  } catch (_) {
    return false;
  }
}

function dismissWorkflowGuide() {
  try {
    localStorage.setItem(WORKFLOW_GUIDE_DISMISSED_KEY, "1");
  } catch (_) {}
  renderWorkflowSheet();
}

function loadCachedWorkflowTemplates() {
  try {
    const raw = localStorage.getItem(WORKFLOW_CACHE_KEY);
    if (raw) {
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed)) {
        return partitionWorkflowTemplates(parsed);
      }
    }
  } catch (_) {}
  return partitionWorkflowTemplates(DEFAULT_WORKFLOW_TEMPLATES);
}

function saveCachedWorkflowTemplates(list) {
  try {
    localStorage.setItem(WORKFLOW_CACHE_KEY, JSON.stringify(partitionWorkflowTemplates(list)));
  } catch (_) {}
}

function isWorkflowSheetOpen() {
  const sheet = document.getElementById("sheet-workflows");
  return Boolean(sheet && !sheet.classList.contains("hidden"));
}

async function fetchWorkflowTemplatesFromGateway() {
  try {
    const resp = await fetch("/gateway/workflow-templates");
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
    const data = await resp.json();
    if (Array.isArray(data.templates)) {
      workflowState.templates = partitionWorkflowTemplates(data.templates);
      workflowState.error = "";
      saveCachedWorkflowTemplates(workflowState.templates);
      if (isWorkflowSheetOpen()) {
        renderWorkflowSheet();
      }
    }
  } catch (err) {
    if (!workflowState.templates.length) {
      workflowState.templates = loadCachedWorkflowTemplates();
    }
    console.warn("[Workflows] Gateway fetch failed, using cached templates:", err);
  }
}

async function syncWorkflowTemplatesToGateway(nextTemplates) {
  const partitioned = partitionWorkflowTemplates(nextTemplates);
  workflowState.templates = partitioned;
  saveCachedWorkflowTemplates(partitioned);
  workflowState.syncing = true;
  workflowState.error = "";
  renderWorkflowSheet();
  try {
    const resp = await fetch("/gateway/workflow-templates", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ templates: partitioned })
    });
    if (!resp.ok) {
      const errData = await resp.json().catch(() => ({}));
      throw new Error(errData.error || `HTTP ${resp.status}`);
    }
    const data = await resp.json();
    if (Array.isArray(data.templates)) {
      workflowState.templates = partitionWorkflowTemplates(data.templates);
      saveCachedWorkflowTemplates(workflowState.templates);
    }
  } catch (err) {
    workflowState.error = "已保存到本机缓存，网关同步失败：" + (err.message || err);
  } finally {
    workflowState.syncing = false;
    renderWorkflowSheet();
  }
}

async function resetWorkflowTemplatesOnGateway() {
  workflowState.syncing = true;
  workflowState.error = "";
  workflowState.confirmReset = false;
  renderWorkflowSheet();
  try {
    const resp = await fetch("/gateway/workflow-templates", { method: "DELETE" });
    if (!resp.ok) {
      const errData = await resp.json().catch(() => ({}));
      throw new Error(errData.error || `HTTP ${resp.status}`);
    }
    const data = await resp.json();
    const list = Array.isArray(data.templates)
      ? partitionWorkflowTemplates(data.templates)
      : partitionWorkflowTemplates(DEFAULT_WORKFLOW_TEMPLATES);
    workflowState.templates = list;
    saveCachedWorkflowTemplates(list);
  } catch (err) {
    workflowState.templates = partitionWorkflowTemplates(DEFAULT_WORKFLOW_TEMPLATES);
    saveCachedWorkflowTemplates(workflowState.templates);
    workflowState.error = "已重置本机列表，网关同步失败：" + (err.message || err);
  } finally {
    workflowState.syncing = false;
    renderWorkflowSheet();
  }
}

function generateWorkflowTemplateId() {
  if (typeof crypto !== "undefined" && crypto.randomUUID) {
    return "wf-" + crypto.randomUUID().slice(0, 8);
  }
  return "wf-" + Date.now().toString(36) + "-" + Math.random().toString(36).slice(2, 6);
}

function openWorkflowSheet(initialEditing = false) {
  dismissWebWorkflowPicker();
  if (typeof dismissWebModelPicker === "function") dismissWebModelPicker();
  const sheet = document.getElementById("sheet-workflows");
  if (!sheet) return;
  if (!workflowState.templates.length) {
    workflowState.templates = loadCachedWorkflowTemplates();
  } else {
    workflowState.templates = partitionWorkflowTemplates(workflowState.templates);
  }
  workflowState.editing = Boolean(initialEditing);
  workflowState.formMode = null;
  workflowState.error = "";
  workflowState.confirmReset = false;
  sheet.classList.remove("hidden");
  renderWorkflowSheet();
  triggerHaptic("light");
  fetchWorkflowTemplatesFromGateway();
}

function closeWorkflowSheet() {
  const sheet = document.getElementById("sheet-workflows");
  if (sheet) sheet.classList.add("hidden");
  workflowState.editing = false;
  workflowState.formMode = null;
  workflowState.confirmReset = false;
}

function applyWorkflowTemplate(promptText) {
  const chatInput = document.getElementById("chat-input");
  if (chatInput) {
    chatInput.value = promptText;
    chatInput.dispatchEvent(new Event("input", { bubbles: true }));
    chatInput.focus();
    const match = /\[[^\[\]\n]+\]/.exec(promptText);
    if (match && typeof chatInput.setSelectionRange === "function") {
      try {
        chatInput.setSelectionRange(match.index, match.index + match[0].length);
      } catch (_) {}
    }
  }
  triggerHaptic("light");
  if (isWorkflowSheetOpen()) {
    closeWorkflowSheet();
  }
}

function openWorkflowForm(mode, item) {
  workflowState.formMode = {
    mode,
    item: item
      ? normalizeWorkflowTemplate(item)
      : {
          id: generateWorkflowTemplateId(),
          title: "",
          subtitle: "",
          icon: "workflow",
          color: "indigo",
          inQuickMenu: false,
          templatePrompt: "/workflow 请使用 run_workflow "
        }
  };
  workflowState.error = "";
  renderWorkflowSheet();
}

function deleteWorkflowTemplate(id) {
  const next = workflowState.templates.filter((t) => t.id !== id);
  triggerHaptic("medium");
  syncWorkflowTemplatesToGateway(next);
}

function renderWorkflowHeader() {
  const leftBtn = document.getElementById("btn-workflows-left");
  const titleEl = document.getElementById("workflows-title");
  const rightBtn = document.getElementById("btn-workflows-right");
  if (!leftBtn || !titleEl || !rightBtn) return;

  const backSvg = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"></polyline></svg>`;
  const checkSvg = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"></polyline></svg>`;
  const pencilSvg = `<svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 20h9"></path><path d="M16.5 3.5a2.121 2.121 0 1 1 3 3L7 19l-4 1 1-4L16.5 3.5z"></path></svg>`;
  const plusSvg = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"></line><line x1="5" y1="12" x2="19" y2="12"></line></svg>`;

  if (workflowState.formMode) {
    leftBtn.innerHTML = backSvg;
    leftBtn.setAttribute("aria-label", "返回");
    leftBtn.classList.remove("primary");
    titleEl.textContent = workflowState.formMode.mode === "add" ? "新增 Workflow" : "编辑 Workflow";
    rightBtn.innerHTML = checkSvg;
    rightBtn.setAttribute("aria-label", "保存");
  } else {
    leftBtn.innerHTML = workflowState.editing ? checkSvg : pencilSvg;
    leftBtn.setAttribute("aria-label", workflowState.editing ? "完成" : "编辑");
    leftBtn.classList.toggle("primary", workflowState.editing);
    titleEl.textContent = "Workflow 模板";
    rightBtn.innerHTML = plusSvg;
    rightBtn.setAttribute("aria-label", "新增 Workflow 模板");
  }
}

function renderWorkflowForm(body) {
  const draft = workflowState.formMode.item;
  const resolvedIcon = normalizeWorkflowIconKey(draft.icon);
  const iconButtons = WORKFLOW_ICON_CATALOG.map((ic) => {
    const selected = resolvedIcon === ic.key;
    const colorHex = workflowColorHex(draft.color);
    return `<button type="button" class="wf-icon-opt ${selected ? "selected" : ""}" data-wf-icon="${ic.key}" title="${ic.label}" style="${selected ? `color:${colorHex};border-color:${colorHex};background:${colorHex}20;` : ""}">
      ${workflowIconSvg(ic.key, 17)}
    </button>`;
  }).join("");

  const colorDots = WORKFLOW_COLOR_CATALOG.map((c) => {
    const selected = draft.color === c.key;
    return `<button type="button" class="wf-color-opt ${selected ? "selected" : ""}" data-wf-color="${c.key}" aria-label="${c.label}" style="background:${c.hex};"></button>`;
  }).join("");

  const errHtml = workflowState.error
    ? `<div class="wf-error-banner">${escapeHtml(workflowState.error)}</div>`
    : "";

  body.innerHTML = `
    <div class="wf-form">
      ${errHtml}
      <div class="wf-field">
        <label>标题</label>
        <input id="wf-input-title" class="wf-form-input" type="text" placeholder="例如：并发安全审计" value="${escapeHtml(draft.title)}" />
      </div>
      <div class="wf-field">
        <label>副标题 / 说明（可选）</label>
        <input id="wf-input-subtitle" class="wf-form-input" type="text" placeholder="一句话说明这个模板适合什么场景" value="${escapeHtml(draft.subtitle)}" />
      </div>
      <div class="wf-field wf-form-toggle-row">
        <div class="wf-form-toggle-copy">
          <div class="wf-form-toggle-title">加入长按快捷菜单</div>
          <div class="wf-form-toggle-sub">开启后，长按底部 Workflow 按钮即可上滑快速选择此模板</div>
        </div>
        <label class="wf-switch" aria-label="加入长按快捷菜单">
          <input id="wf-input-quick" type="checkbox" ${draft.inQuickMenu ? "checked" : ""} />
          <span class="wf-switch-slider"></span>
        </label>
      </div>
      <div class="wf-field">
        <label>图标</label>
        <div class="wf-icon-grid">${iconButtons}</div>
      </div>
      <div class="wf-field">
        <label>主题色</label>
        <div class="wf-color-row">${colorDots}</div>
      </div>
      <div class="wf-field">
        <label>模板内容（点选后直接填充到输入框）</label>
        <textarea id="wf-input-prompt" class="wf-form-textarea" rows="5" placeholder="支持任意提示词或 /workflow、/plan 等斜杠命令，可用 [占位符] 提示自己补充参数">${escapeHtml(draft.templatePrompt)}</textarea>
      </div>
    </div>
  `;

  body.querySelector("#wf-input-title")?.addEventListener("input", (e) => {
    workflowState.formMode.item.title = e.target.value;
  });
  body.querySelector("#wf-input-subtitle")?.addEventListener("input", (e) => {
    workflowState.formMode.item.subtitle = e.target.value;
  });
  body.querySelector("#wf-input-quick")?.addEventListener("change", (e) => {
    workflowState.formMode.item.inQuickMenu = Boolean(e.target.checked);
  });
  body.querySelector("#wf-input-prompt")?.addEventListener("input", (e) => {
    workflowState.formMode.item.templatePrompt = e.target.value;
  });
  body.querySelectorAll(".wf-icon-opt").forEach((btn) => {
    btn.addEventListener("click", () => {
      workflowState.formMode.item.icon = btn.dataset.wfIcon;
      renderWorkflowSheet();
    });
  });
  body.querySelectorAll(".wf-color-opt").forEach((btn) => {
    btn.addEventListener("click", () => {
      workflowState.formMode.item.color = btn.dataset.wfColor;
      renderWorkflowSheet();
    });
  });
}

function submitWorkflowForm() {
  if (!workflowState.formMode) return;
  const item = workflowState.formMode.item;
  const title = (item.title || "").trim();
  const prompt = (item.templatePrompt || "").trim();
  if (!title) {
    workflowState.error = "请填写模板标题";
    renderWorkflowSheet();
    return;
  }
  if (!prompt) {
    workflowState.error = "请填写提示词或命令模板内容";
    renderWorkflowSheet();
    return;
  }
  const normalized = normalizeWorkflowTemplate({
    ...item,
    title,
    subtitle: (item.subtitle || "").trim() || "自定义工作流命令",
    templatePrompt: prompt
  });
  let next;
  if (workflowState.formMode.mode === "add") {
    next = [...workflowState.templates, normalized];
  } else {
    next = workflowState.templates.map((t) => (t.id === normalized.id ? normalized : t));
  }
  workflowState.formMode = null;
  triggerHaptic("medium");
  syncWorkflowTemplatesToGateway(next);
}

function buildWorkflowGuideHtml() {
  if (isWorkflowGuideDismissed()) return "";
  const miniSvg = `<svg width="48" height="44" viewBox="0 0 48 44" fill="none" aria-hidden="true">
    <rect x="6" y="4" width="36" height="18" rx="5" fill="rgba(255,255,255,0.08)" stroke="rgba(255,255,255,0.22)" stroke-width="1"/>
    <line x1="11" y1="10" x2="31" y2="10" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" opacity="0.9"/>
    <line x1="11" y1="16" x2="25" y2="16" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" opacity="0.5"/>
    <path d="M24 30 L24 23 M21.5 25.5 L24 23 L26.5 25.5" stroke="#0a84ff" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/>
    <rect x="8" y="32" width="32" height="10" rx="5" fill="rgba(255,255,255,0.06)" stroke="rgba(255,255,255,0.2)" stroke-width="1"/>
    <text x="24" y="39" text-anchor="middle" fill="currentColor" font-size="6" font-weight="600" opacity="0.85">Workflow</text>
  </svg>`;
  const actionBtn = workflowState.editing
    ? ""
    : `<button id="btn-wf-guide-config" class="wf-guide-action-btn" type="button">去配置 ›</button>`;
  return `
    <div class="wf-guide-card">
      <div class="wf-guide-visual">${miniSvg}</div>
      <div class="wf-guide-content">
        <div class="wf-guide-title">长按 Workflow 按钮，上滑秒选模板</div>
        <div class="wf-guide-sub">编辑模式下按住左侧 ≡ 手柄，可把模板拖进下方「长按快捷菜单」。</div>
        ${actionBtn}
      </div>
      <button id="btn-wf-guide-dismiss" class="wf-guide-dismiss" type="button" aria-label="关闭引导">×</button>
    </div>
  `;
}

function renderWorkflowCardHtml(item) {
  const hex = workflowColorHex(item.color);
  const sub = item.subtitle ? `<div class="wf-card-sub">${escapeHtml(item.subtitle)}</div>` : "";
  const dragHandle = workflowState.editing
    ? `<div class="wf-drag-handle" title="按住拖拽排序或跨区移动" aria-label="拖拽排序">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <line x1="4" y1="7" x2="20" y2="7"></line>
          <line x1="4" y1="12" x2="20" y2="12"></line>
          <line x1="4" y1="17" x2="20" y2="17"></line>
        </svg>
      </div>`
    : "";
  // 编辑模式下移除右侧编辑铅笔按钮与 +/- 按钮，短按卡片直接进入编辑
  const rightSlot = workflowState.editing
    ? ""
    : `<span class="wf-chevron">›</span>`;

  return `
    <div class="wf-swipe-row" data-wf-id="${escapeHtml(item.id)}" data-in-quick="${item.inQuickMenu ? "1" : "0"}">
      <div class="wf-swipe-actions">
        <button type="button" class="wf-swipe-del-btn" data-wf-del="${escapeHtml(item.id)}" aria-label="删除">
          <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"></polyline><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"></path><path d="M10 11v6"></path><path d="M14 11v6"></path><path d="M9 6V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2"></path></svg>
          <span>删除</span>
        </button>
      </div>
      <div class="wf-card ${workflowState.editing ? "editing" : ""}" data-wf-id="${escapeHtml(item.id)}" data-wf-pick="${escapeHtml(item.id)}">
        ${dragHandle}
        <div class="wf-card-icon" style="background:${hex}1f;color:${hex};">
          ${workflowIconSvg(item.icon, 18)}
        </div>
        <div class="wf-card-body">
          <div class="wf-card-title">${escapeHtml(item.title)}</div>
          ${sub}
        </div>
        ${rightSlot}
      </div>
    </div>
  `;
}

function bindWorkflowDragReorder(containerEl) {
  if (!workflowState.editing || !containerEl) return;
  const handles = Array.from(containerEl.querySelectorAll(".wf-drag-handle"));

  handles.forEach((handle) => {
    handle.addEventListener("pointerdown", (startEvent) => {
      if (startEvent.button !== 0 && startEvent.pointerType === "mouse") return;
      startEvent.preventDefault();
      startEvent.stopPropagation();

      const dragRow = handle.closest(".wf-swipe-row");
      const quickListEl = containerEl.querySelector("#wf-quick-list");
      const moreListEl = containerEl.querySelector("#wf-more-list");
      const boundaryEl = containerEl.querySelector("#wf-more-header");
      const quickDropEl = containerEl.querySelector("#wf-quick-dropzone");
      const moreDropEl = containerEl.querySelector("#wf-more-dropzone");
      if (!dragRow || !quickListEl || !moreListEl || !boundaryEl) return;

      closeAllSwipedWorkflowCards(containerEl, null);

      const quickRows = Array.from(quickListEl.querySelectorAll(".wf-swipe-row"));
      const moreRows = Array.from(moreListEl.querySelectorAll(".wf-swipe-row"));
      const startQuick = dragRow.dataset.inQuick === "1";
      const sourceRows = startQuick ? quickRows : moreRows;
      const origSectionIdx = sourceRows.indexOf(dragRow);
      if (origSectionIdx < 0) return;

      const snapshotMeta = (rows) =>
        rows.map((el) => {
          const r = el.getBoundingClientRect();
          return {
            el,
            height: r.height,
            centerY: r.top + r.height / 2
          };
        });

      const quickMeta = snapshotMeta(quickRows);
      const moreMeta = snapshotMeta(moreRows);
      const startRowRect = dragRow.getBoundingClientRect();
      const dragDelta = startRowRect.height + 10;
      const startCenterY = startRowRect.top + startRowRect.height / 2;
      const boundaryY = boundaryEl.getBoundingClientRect().top;

      let targetInQuick = startQuick;
      let targetSectionIdx = origSectionIdx;
      const pointerId = startEvent.pointerId;
      const startClientY = startEvent.clientY;

      dragRow.classList.add("is-dragging");
      dragRow.style.zIndex = "40";
      dragRow.style.transition = "none";
      try {
        handle.setPointerCapture(pointerId);
      } catch (_) {}
      triggerHaptic("light");

      const applySectionShifts = (metaList, targetIdx, isSourceSection, isTargetSection) => {
        metaList.forEach((m, idx) => {
          if (m.el === dragRow) return;
          m.el.style.transition = "transform 160ms ease";
          let shift = 0;
          if (isSourceSection && isTargetSection) {
            if (origSectionIdx < targetIdx && idx > origSectionIdx && idx <= targetIdx) {
              shift = -dragDelta;
            } else if (origSectionIdx > targetIdx && idx >= targetIdx && idx < origSectionIdx) {
              shift = dragDelta;
            }
          } else if (isSourceSection && !isTargetSection) {
            if (idx > origSectionIdx) shift = -dragDelta;
          } else if (!isSourceSection && isTargetSection) {
            if (idx >= targetIdx) shift = dragDelta;
          }
          m.el.style.transform = shift ? `translateY(${shift}px)` : "";
        });
      };

      const onMove = (moveEvent) => {
        if (moveEvent.pointerId !== pointerId) return;
        const dy = moveEvent.clientY - startClientY;
        dragRow.style.transform = `translateY(${dy}px) scale(1.015)`;

        const draggedCenterY = startCenterY + dy;
        const pointerY = moveEvent.clientY;
        const nextQuick = pointerY < boundaryY;

        const candidateMeta = (nextQuick ? quickMeta : moreMeta).filter((m) => m.el !== dragRow);
        let nextIdx = 0;
        for (let i = 0; i < candidateMeta.length; i++) {
          if (candidateMeta[i].centerY < draggedCenterY) {
            nextIdx = i + 1;
          }
        }

        if (nextQuick !== targetInQuick || nextIdx !== targetSectionIdx) {
          targetInQuick = nextQuick;
          targetSectionIdx = nextIdx;
          triggerHaptic("light");
        }

        quickDropEl?.classList.toggle("drag-over", targetInQuick);
        moreDropEl?.classList.toggle("drag-over", !targetInQuick);
        applySectionShifts(quickMeta, targetInQuick ? targetSectionIdx : 0, startQuick, targetInQuick);
        applySectionShifts(moreMeta, !targetInQuick ? targetSectionIdx : 0, !startQuick, !targetInQuick);
      };

      const cleanup = () => {
        try {
          handle.releasePointerCapture(pointerId);
        } catch (_) {}
        window.removeEventListener("pointermove", onMove);
        window.removeEventListener("pointerup", onEnd);
        window.removeEventListener("pointercancel", onEnd);
        dragRow.classList.remove("is-dragging");
        dragRow.style.zIndex = "";
        dragRow.style.transition = "";
        dragRow.style.transform = "";
        quickDropEl?.classList.remove("drag-over");
        moreDropEl?.classList.remove("drag-over");
        [...quickRows, ...moreRows].forEach((r) => {
          r.style.transition = "";
          r.style.transform = "";
        });
      };

      const onEnd = (endEvent) => {
        if (endEvent.pointerId !== pointerId) return;
        const changed = targetInQuick !== startQuick || targetSectionIdx !== origSectionIdx;
        const draggedId = dragRow.dataset.wfId;
        cleanup();
        if (!changed || !draggedId) return;

        const allPartitioned = partitionWorkflowTemplates(workflowState.templates);
        const currentQuick = allPartitioned.filter((t) => t.inQuickMenu);
        const currentMore = allPartitioned.filter((t) => !t.inQuickMenu);
        const srcArr = startQuick ? currentQuick : currentMore;
        const removeIdx = srcArr.findIndex((t) => t.id === draggedId);
        if (removeIdx < 0) return;

        const [movedRaw] = srcArr.splice(removeIdx, 1);
        const moved = { ...movedRaw, inQuickMenu: targetInQuick };
        const dstArr = targetInQuick ? currentQuick : currentMore;
        const clampedIdx = Math.max(0, Math.min(targetSectionIdx, dstArr.length));
        dstArr.splice(clampedIdx, 0, moved);

        syncWorkflowTemplatesToGateway([...currentQuick, ...currentMore]);
      };

      window.addEventListener("pointermove", onMove);
      window.addEventListener("pointerup", onEnd);
      window.addEventListener("pointercancel", onEnd);
    });
  });
}

function closeAllSwipedWorkflowCards(containerEl, exceptWrapper) {
  if (!containerEl) return;
  containerEl.querySelectorAll(".wf-swipe-row.is-swiped").forEach((w) => {
    if (w !== exceptWrapper) {
      w.classList.remove("is-swiped");
      const card = w.querySelector(".wf-card");
      if (card) card.style.transform = "";
    }
  });
}

function bindWorkflowSwipeDelete(containerEl) {
  if (!containerEl) return;
  const rows = containerEl.querySelectorAll(".wf-swipe-row");
  rows.forEach((wrapper) => {
    const card = wrapper.querySelector(".wf-card");
    if (!card) return;
    let startX = 0;
    let startY = 0;
    let swiping = false;
    let baseOffset = 0;
    let currentOffset = 0;

    card.addEventListener("touchstart", (e) => {
      if (e.target.closest(".wf-drag-handle")) return;
      if (e.touches.length !== 1) return;
      startX = e.touches[0].clientX;
      startY = e.touches[0].clientY;
      swiping = false;
      baseOffset = wrapper.classList.contains("is-swiped") ? -76 : 0;
      currentOffset = baseOffset;
      closeAllSwipedWorkflowCards(containerEl, wrapper);
    }, { passive: true });

    card.addEventListener("touchmove", (e) => {
      if (e.target.closest(".wf-drag-handle")) return;
      if (e.touches.length !== 1) return;
      const dx = e.touches[0].clientX - startX;
      const dy = e.touches[0].clientY - startY;
      if (!swiping && Math.abs(dx) > 10 && Math.abs(dx) > Math.abs(dy) * 1.3) {
        swiping = true;
      }
      if (!swiping) return;
      currentOffset = Math.max(-88, Math.min(0, baseOffset + dx));
      card.style.transition = "none";
      card.style.transform = `translateX(${currentOffset}px)`;
    }, { passive: true });

    card.addEventListener("touchend", () => {
      if (!swiping) return;
      card.style.transition = "";
      if (currentOffset <= -42) {
        wrapper.classList.add("is-swiped");
        card.style.transform = "translateX(-76px)";
      } else {
        wrapper.classList.remove("is-swiped");
        card.style.transform = "";
      }
      card.dataset.justSwiped = "1";
      setTimeout(() => delete card.dataset.justSwiped, 160);
    });
  });
}

function renderWorkflowSheet() {
  renderWorkflowHeader();
  const body = document.getElementById("workflows-body");
  if (!body) return;

  if (workflowState.formMode) {
    renderWorkflowForm(body);
    return;
  }

  const errHtml = workflowState.error
    ? `<div class="wf-error-banner">${escapeHtml(workflowState.error)}</div>`
    : "";

  const list = partitionWorkflowTemplates(workflowState.templates);
  if (!list.length) {
    body.innerHTML = `
      ${errHtml}
      <div class="wf-empty-state">
        <p>暂无 Workflow 模板</p>
        <div class="wf-empty-actions">
          <button id="btn-wf-empty-add" class="wf-btn-primary" type="button">新增模板</button>
          <button id="btn-wf-empty-reset" class="wf-btn-ghost" type="button">恢复默认</button>
        </div>
      </div>
    `;
    body.querySelector("#btn-wf-empty-add")?.addEventListener("click", () => openWorkflowForm("add"));
    body.querySelector("#btn-wf-empty-reset")?.addEventListener("click", () => resetWorkflowTemplatesOnGateway());
    return;
  }

  const quickItems = list.filter((item) => item.inQuickMenu);
  const moreItems = list.filter((item) => !item.inQuickMenu);

  const quickListHtml = quickItems.length
    ? quickItems.map(renderWorkflowCardHtml).join("")
    : `<div id="wf-quick-dropzone" class="wf-empty-dropzone ${workflowState.editing ? "" : "is-tappable"}">
        ${workflowState.editing ? "按住下方模板左侧 ≡ 手柄拖拽至此" : "暂未配置 · 点击进入编辑后将下方模板拖入此处"}
      </div>`;

  const moreListHtml = moreItems.length
    ? moreItems.map(renderWorkflowCardHtml).join("")
    : workflowState.editing
      ? `<div id="wf-more-dropzone" class="wf-empty-dropzone">拖至此处可移出长按快捷菜单</div>`
      : `<div class="wf-more-all-hint">全部模板已加入长按快捷菜单</div>`;

  let resetHtml = "";
  if (workflowState.editing) {
    if (workflowState.confirmReset) {
      resetHtml = `
        <div class="wf-reset-confirm">
          <span>恢复默认将清空自定义模板，确定吗？</span>
          <div class="wf-reset-confirm-btns">
            <button id="btn-wf-reset-cancel" class="wf-btn-ghost" type="button">取消</button>
            <button id="btn-wf-reset-confirm" class="wf-btn-danger" type="button">确认恢复</button>
          </div>
        </div>
      `;
    } else {
      resetHtml = `
        <div class="wf-reset-area">
          <button id="btn-wf-reset-ask" class="wf-btn-danger-outline" type="button">恢复默认模板列表</button>
        </div>
      `;
    }
  }

  body.innerHTML = `
    ${errHtml}
    ${buildWorkflowGuideHtml()}
    <div id="workflows-list" class="wf-sections-root">
      <div id="wf-quick-section" class="wf-section">
        <div class="wf-section-header">
          <span class="wf-section-title">长按快捷菜单</span>
          <span class="wf-section-badge">${quickItems.length}</span>
          ${workflowState.editing ? '<span class="wf-section-hint">拖至此区域置顶</span>' : ""}
        </div>
        <div id="wf-quick-list" class="wf-list">${quickListHtml}</div>
      </div>
      <div id="wf-more-header" class="wf-section-header wf-section-boundary">
        <span class="wf-section-title">更多模板</span>
        <span class="wf-section-badge">${moreItems.length}</span>
        ${workflowState.editing ? '<span class="wf-section-hint">短按编辑 · 左滑删除</span>' : ""}
      </div>
      <div id="wf-more-section" class="wf-section">
        <div id="wf-more-list" class="wf-list">${moreListHtml}</div>
      </div>
    </div>
    ${resetHtml}
  `;

  const listRoot = body.querySelector("#workflows-list");
  bindWorkflowSwipeDelete(listRoot);

  body.querySelector("#btn-wf-guide-dismiss")?.addEventListener("click", (e) => {
    e.stopPropagation();
    dismissWorkflowGuide();
  });
  body.querySelector("#btn-wf-guide-config")?.addEventListener("click", (e) => {
    e.stopPropagation();
    workflowState.editing = true;
    workflowState.confirmReset = false;
    renderWorkflowSheet();
  });
  body.querySelector("#wf-quick-dropzone.is-tappable")?.addEventListener("click", () => {
    workflowState.editing = true;
    workflowState.confirmReset = false;
    renderWorkflowSheet();
  });

  body.querySelectorAll("[data-wf-del]").forEach((btn) => {
    btn.addEventListener("click", (e) => {
      e.stopPropagation();
      deleteWorkflowTemplate(btn.dataset.wfDel);
    });
  });

  if (workflowState.editing) {
    bindWorkflowDragReorder(listRoot);
    body.querySelectorAll(".wf-card.editing").forEach((card) => {
      card.addEventListener("click", (e) => {
        if (e.target.closest(".wf-drag-handle")) return;
        if (card.dataset.justSwiped) return;
        const wrapper = card.closest(".wf-swipe-row");
        if (wrapper && wrapper.classList.contains("is-swiped")) {
          wrapper.classList.remove("is-swiped");
          card.style.transform = "";
          return;
        }
        const found = workflowState.templates.find((t) => t.id === card.dataset.wfId);
        if (found) openWorkflowForm("edit", found);
      });
    });
    body.querySelector("#btn-wf-reset-ask")?.addEventListener("click", () => {
      workflowState.confirmReset = true;
      renderWorkflowSheet();
    });
    body.querySelector("#btn-wf-reset-cancel")?.addEventListener("click", () => {
      workflowState.confirmReset = false;
      renderWorkflowSheet();
    });
    body.querySelector("#btn-wf-reset-confirm")?.addEventListener("click", () => {
      resetWorkflowTemplatesOnGateway();
    });
  } else {
    body.querySelectorAll(".wf-card[data-wf-pick]").forEach((card) => {
      card.addEventListener("click", () => {
        if (card.dataset.justSwiped) return;
        const wrapper = card.closest(".wf-swipe-row");
        if (wrapper && wrapper.classList.contains("is-swiped")) {
          wrapper.classList.remove("is-swiped");
          card.style.transform = "";
          return;
        }
        const found = workflowState.templates.find((t) => t.id === card.dataset.wfPick);
        if (found) applyWorkflowTemplate(found.templatePrompt);
      });
    });
  }
}

// ---------- 长按底部 Workflow 按钮上滑快捷输入浮层 ----------

function dismissWebWorkflowPicker() {
  const overlay = document.getElementById("workflow-picker-overlay");
  if (overlay) {
    overlay.classList.add("hidden");
    overlay.innerHTML = "";
  }
  webWorkflowPickerState = null;
}

function openWebWorkflowPicker(anchorEl, options = {}) {
  if (typeof dismissWebModelPicker === "function") dismissWebModelPicker();
  const overlay = document.getElementById("workflow-picker-overlay");
  if (!overlay || !anchorEl) return;

  const allTemplates = workflowState.templates.length
    ? partitionWorkflowTemplates(workflowState.templates)
    : loadCachedWorkflowTemplates();
  const quickItems = allTemplates.filter((t) => t.inQuickMenu);
  const isEmpty = quickItems.length === 0;
  const interactiveTapMode = Boolean(options.interactiveTapMode);

  webWorkflowPickerState = {
    quickItems,
    isEmpty,
    interactiveTapMode,
    hoveredIndex: null,
    hasEnteredMenu: false,
    anchorRect: anchorEl.getBoundingClientRect()
  };

  renderWebWorkflowPicker();
  overlay.classList.remove("hidden");
  fetchWorkflowTemplatesFromGateway().then(() => {
    if (!webWorkflowPickerState) return;
    const latestQuick = partitionWorkflowTemplates(workflowState.templates).filter((t) => t.inQuickMenu);
    if (
      latestQuick.length !== webWorkflowPickerState.quickItems.length ||
      latestQuick.some((item, i) => item.id !== webWorkflowPickerState.quickItems[i]?.id || item.title !== webWorkflowPickerState.quickItems[i]?.title)
    ) {
      webWorkflowPickerState.quickItems = latestQuick;
      webWorkflowPickerState.isEmpty = latestQuick.length === 0;
      renderWebWorkflowPicker();
    }
  });
}

function renderWebWorkflowPicker() {
  const overlay = document.getElementById("workflow-picker-overlay");
  const st = webWorkflowPickerState;
  if (!overlay || !st) return;

  const vw = window.innerWidth;
  const vh = window.innerHeight;
  const cardWidth = st.isEmpty
    ? Math.min(276, Math.max(236, vw - 24))
    : Math.min(264, Math.max(204, vw - 24));
  const cardLeft = Math.max(12, Math.min(st.anchorRect.left, vw - cardWidth - 12));
  const maxHeight = Math.max(72, st.anchorRect.top - 20);
  const cardBottom = Math.max(16, vh - st.anchorRect.top + 8);

  const rowsHtml = st.isEmpty
    ? `<button type="button" class="mp-row wf-picker-row wf-picker-empty-row ${st.hoveredIndex === 0 ? "is-hovered" : ""}" data-wf-quick-idx="0" data-wf-setup="1">
        <span class="mp-title">暂未配置快捷模板 · 点击去设置</span>
        <span class="mp-arrow">›</span>
      </button>`
    : st.quickItems
        .map(
          (item, idx) => `<button type="button" class="mp-row wf-picker-row ${st.hoveredIndex === idx ? "is-hovered" : ""}" data-wf-quick-idx="${idx}">
            <span class="mp-title">${escapeHtml(item.title)}</span>
          </button>`
        )
        .join("");

  overlay.innerHTML = `
    <div class="mp-backdrop" ${st.interactiveTapMode ? "" : 'style="background:rgba(0,0,0,0.03);"'}></div>
    <div class="mp-card wf-picker-card" style="left:${cardLeft}px;bottom:${cardBottom}px;width:${cardWidth}px;max-height:${maxHeight}px;">
      ${rowsHtml}
    </div>
  `;

  overlay.querySelector(".mp-backdrop")?.addEventListener("click", dismissWebWorkflowPicker);
  overlay.querySelectorAll(".wf-picker-row").forEach((row) => {
    row.addEventListener("click", () => {
      if (row.dataset.wfSetup === "1" || st.isEmpty) {
        dismissWebWorkflowPicker();
        openWorkflowSheet(true);
        return;
      }
      const idx = Number(row.dataset.wfQuickIdx);
      const picked = st.quickItems[idx];
      dismissWebWorkflowPicker();
      if (picked) {
        applyWorkflowTemplate(picked.templatePrompt);
      }
    });
  });
}

function updateWebWorkflowPickerHover(clientX, clientY) {
  const overlay = document.getElementById("workflow-picker-overlay");
  const st = webWorkflowPickerState;
  if (!overlay || !st || overlay.classList.contains("hidden")) return;

  const card = overlay.querySelector(".wf-picker-card");
  if (!card) return;
  const cardRect = card.getBoundingClientRect();
  const rows = Array.from(overlay.querySelectorAll(".wf-picker-row"));

  let nextHovered = null;
  if (
    clientX >= cardRect.left - 28 &&
    clientX <= cardRect.right + 28 &&
    clientY >= cardRect.top - 24 &&
    clientY <= cardRect.bottom + 6
  ) {
    st.hasEnteredMenu = true;
    let bestDist = Infinity;
    rows.forEach((row, idx) => {
      const r = row.getBoundingClientRect();
      if (clientY >= r.top && clientY <= r.bottom) {
        nextHovered = idx;
        bestDist = 0;
      } else {
        const dist = clientY < r.top ? r.top - clientY : clientY - r.bottom;
        if (dist < bestDist) {
          bestDist = dist;
          nextHovered = idx;
        }
      }
    });
  }

  if (st.hoveredIndex !== nextHovered) {
    st.hoveredIndex = nextHovered;
    rows.forEach((r, idx) => r.classList.toggle("is-hovered", idx === nextHovered));
    if (nextHovered !== null) {
      triggerHaptic("light");
    }
  }
}

function bindWorkflowChipButton() {
  const btn = document.getElementById("btn-workflow-templates");
  if (!btn) return;

  let pressTimer = null;
  let activePointerId = null;
  let startX = 0;
  let startY = 0;
  let longPressTriggered = false;
  let suppressNextClick = false;

  const clearTimer = () => {
    if (pressTimer !== null) {
      clearTimeout(pressTimer);
      pressTimer = null;
    }
  };

  const cleanupWindowListeners = () => {
    window.removeEventListener("pointermove", onPointerMove);
    window.removeEventListener("pointerup", onPointerUp);
    window.removeEventListener("pointercancel", onPointerCancel);
    window.removeEventListener("touchmove", onTouchMovePreventScroll);
  };

  const onTouchMovePreventScroll = (e) => {
    if (longPressTriggered) {
      e.preventDefault();
    }
  };

  const onPointerMove = (e) => {
    if (e.pointerId !== activePointerId) return;
    if (!longPressTriggered) {
      if (Math.hypot(e.clientX - startX, e.clientY - startY) > 14) {
        clearTimer();
      }
      return;
    }
    updateWebWorkflowPickerHover(e.clientX, e.clientY);
  };

  const onPointerEnd = (e) => {
    if (e.pointerId !== activePointerId) return;
    clearTimer();
    cleanupWindowListeners();
    activePointerId = null;

    if (!longPressTriggered) return;
    longPressTriggered = false;
    suppressNextClick = true;
    setTimeout(() => {
      suppressNextClick = false;
    }, 320);

    const st = webWorkflowPickerState;
    if (!st) return;

    if (st.hoveredIndex !== null) {
      if (st.isEmpty) {
        dismissWebWorkflowPicker();
        openWorkflowSheet(true);
      } else {
        const picked = st.quickItems[st.hoveredIndex];
        dismissWebWorkflowPicker();
        if (picked) applyWorkflowTemplate(picked.templatePrompt);
      }
      return;
    }

    if (!st.hasEnteredMenu) {
      st.interactiveTapMode = true;
      renderWebWorkflowPicker();
    } else {
      dismissWebWorkflowPicker();
    }
  };

  const onPointerUp = onPointerEnd;
  const onPointerCancel = onPointerEnd;

  btn.addEventListener("pointerdown", (e) => {
    if (e.pointerType === "mouse" && e.button !== 0) return;
    if (webWorkflowPickerState) return;
    clearTimer();
    cleanupWindowListeners();

    activePointerId = e.pointerId;
    startX = e.clientX;
    startY = e.clientY;
    longPressTriggered = false;

    window.addEventListener("pointermove", onPointerMove);
    window.addEventListener("pointerup", onPointerUp);
    window.addEventListener("pointercancel", onPointerCancel);
    window.addEventListener("touchmove", onTouchMovePreventScroll, { passive: false });

    pressTimer = setTimeout(() => {
      pressTimer = null;
      if (activePointerId === null) return;
      longPressTriggered = true;
      openWebWorkflowPicker(btn, { interactiveTapMode: false });
      triggerHaptic("medium");
    }, 220);
  });

  btn.addEventListener("contextmenu", (e) => {
    e.preventDefault();
    if (!longPressTriggered && !webWorkflowPickerState) {
      openWebWorkflowPicker(btn, { interactiveTapMode: true });
      triggerHaptic("light");
    }
  });

  btn.addEventListener("click", (e) => {
    if (suppressNextClick) {
      e.preventDefault();
      e.stopPropagation();
      suppressNextClick = false;
      return;
    }
    if (webWorkflowPickerState) {
      dismissWebWorkflowPicker();
      return;
    }
    openWorkflowSheet(false);
  });
}

function initWorkflowTemplates() {
  workflowState.templates = loadCachedWorkflowTemplates();
  fetchWorkflowTemplatesFromGateway();

  bindWorkflowChipButton();

  const sheet = document.getElementById("sheet-workflows");
  if (sheet) {
    sheet.addEventListener("click", (e) => {
      if (e.target === sheet) closeWorkflowSheet();
    });
    enableSheetPullToDismiss(sheet, closeWorkflowSheet);
  }

  document.getElementById("btn-workflows-left")?.addEventListener("click", () => {
    if (workflowState.formMode) {
      workflowState.formMode = null;
      workflowState.error = "";
      renderWorkflowSheet();
    } else {
      workflowState.editing = !workflowState.editing;
      workflowState.confirmReset = false;
      renderWorkflowSheet();
    }
  });

  document.getElementById("btn-workflows-right")?.addEventListener("click", () => {
    if (workflowState.formMode) {
      submitWorkflowForm();
    } else {
      openWorkflowForm("add");
    }
  });
}
