// Workflow 模板面板（Web 端对齐 iOS/Android WorkflowTemplateSheet，支持自定义增删改、拖拽排序、左滑删除与网关同步）

const WORKFLOW_CACHE_KEY = "agy_workflow_templates_v1";

const DEFAULT_WORKFLOW_TEMPLATES = [
  {
    id: "code_review",
    title: "并发代码审查",
    subtitle: "并行启动安全、性能、类型规范多角度 Sub-Agents 进行深度审计",
    icon: "security",
    color: "indigo",
    templatePrompt:
      "/workflow 请使用 run_workflow 动态编排工作流，以 parallel 并发对 [模块/文件路径] 进行多维度审计（安全/性能/类型规范），并在最终阶段输出结构化修复建议。"
  },
  {
    id: "test_and_fix",
    title: "并发单测与故障修复",
    subtitle: "并行运行各模块单元测试，定位失败用例并尝试由子代理生成补丁",
    icon: "bug",
    color: "green",
    templatePrompt:
      "/workflow 请使用 run_workflow 编排自动化单测，以 parallel 同时运行 [测试目标或路径] 的单元测试，聚合失败信息并生成修复方案。"
  },
  {
    id: "research",
    title: "多维度综合调研",
    subtitle: "分派多个子代理从技术实现、竞品方案、生产实践分头研究并归纳",
    icon: "search",
    color: "blue",
    templatePrompt:
      "/workflow 请使用 run_workflow 编排调研工作流，分派子代理分别调查 [调研主题] 的核心技术方案、竞品对比与最佳实践，并汇总为结构化报告。"
  },
  {
    id: "custom",
    title: "自定义动态工作流",
    subtitle: "自由描述任务，让 Agent 依据依赖自主规划 parallel / pipeline 阶段",
    icon: "workflow",
    color: "purple",
    templatePrompt:
      "/workflow 请使用 run_workflow 编写 Python 动态工作流脚本，分阶段执行以下任务：[描述具体任务]，要求使用 parallel 并发加速并给出 finish 契约结果。"
  }
];

const WORKFLOW_ICON_CATALOG = [
  { key: "workflow", label: "工作流" },
  { key: "security", label: "安全审查" },
  { key: "bug", label: "测试修复" },
  { key: "search", label: "调研分析" },
  { key: "bolt", label: "快捷执行" },
  { key: "code", label: "代码开发" },
  { key: "terminal", label: "终端脚本" },
  { key: "doc", label: "文档总结" },
  { key: "star", label: "常用收藏" },
  { key: "sparkles", label: "AI 增强" },
  { key: "git", label: "Git 分支" },
  { key: "merge", label: "分支合并" },
  { key: "pipeline", label: "流水线" },
  { key: "checklist", label: "流程清单" },
  { key: "report", label: "分析报告" }
];

const WORKFLOW_COLOR_CATALOG = [
  { key: "indigo", hex: "#5856d6", label: "靛蓝" },
  { key: "green", hex: "#34c759", label: "翠绿" },
  { key: "blue", hex: "#0a84ff", label: "湛蓝" },
  { key: "purple", hex: "#af52de", label: "亮紫" },
  { key: "orange", hex: "#ff9500", label: "活力橙" },
  { key: "red", hex: "#ff3b30", label: "珊瑚红" },
  { key: "teal", hex: "#30b0c7", label: "青碧" }
];

const workflowState = {
  templates: [],
  editing: false,
  formMode: null, // null | { mode: "add" | "edit", item: {...} }
  syncing: false,
  error: "",
  confirmReset: false
};

function workflowColorHex(colorKey) {
  const found = WORKFLOW_COLOR_CATALOG.find((c) => c.key === colorKey);
  return found ? found.hex : "#5856d6";
}

function workflowIconSvg(iconKey, size = 18) {
  const s = `width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round"`;
  switch (iconKey) {
    case "security":
      return `<svg ${s}><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"></path><path d="M12 8v8"></path></svg>`;
    case "bug":
      return `<svg ${s}><path d="m8 2 1.88 1.88"></path><path d="M14.12 3.88 16 2"></path><path d="M9 7.13v-1a3.003 3.003 0 1 1 6 0v1"></path><path d="M12 20c-3.3 0-6-2.7-6-6v-3a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v3c0 3.3-2.7 6-6 6"></path><path d="M12 20v-9"></path><path d="M6.53 9C4.6 8.8 3 7.1 3 5"></path><path d="M6 13H2"></path><path d="M3 21c0-2.1 1.7-3.9 3.8-4"></path><path d="M20.97 5c0 2.1-1.6 3.8-3.5 4"></path><path d="M22 13h-4"></path><path d="M17.2 17c2.1.1 3.8 1.9 3.8 4"></path></svg>`;
    case "search":
      return `<svg ${s}><circle cx="11" cy="11" r="8"></circle><line x1="21" y1="21" x2="16.65" y2="16.65"></line></svg>`;
    case "bolt":
      return `<svg ${s}><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"></polygon></svg>`;
    case "code":
      return `<svg ${s}><polyline points="16 18 22 12 16 6"></polyline><polyline points="8 6 2 12 8 18"></polyline></svg>`;
    case "terminal":
      return `<svg ${s}><polyline points="4 17 10 11 4 5"></polyline><line x1="12" y1="19" x2="20" y2="19"></line></svg>`;
    case "doc":
      return `<svg ${s}><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline><line x1="16" y1="13" x2="8" y2="13"></line><line x1="16" y1="17" x2="8" y2="17"></line></svg>`;
    case "star":
      return `<svg ${s}><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"></polygon></svg>`;
    case "sparkles":
      return `<svg ${s}><path d="m12 3-1.9 5.8a2 2 0 0 1-1.3 1.3L3 12l5.8 1.9a2 2 0 0 1 1.3 1.3L12 21l1.9-5.8a2 2 0 0 1 1.3-1.3L21 12l-5.8-1.9a2 2 0 0 1-1.3-1.3Z"></path></svg>`;
    case "git":
      return `<svg ${s}><line x1="6" y1="3" x2="6" y2="15"></line><circle cx="18" cy="6" r="3"></circle><circle cx="6" cy="18" r="3"></circle><path d="M18 9a9 9 0 0 1-9 9"></path></svg>`;
    case "merge":
      return `<svg ${s}><circle cx="18" cy="18" r="3"></circle><circle cx="6" cy="6" r="3"></circle><path d="M6 21V9a9 9 0 0 0 9 9"></path></svg>`;
    case "pipeline":
      return `<svg ${s}><path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8"></path><path d="M21 3v5h-5"></path></svg>`;
    case "checklist":
      return `<svg ${s}><path d="m3 7 2 2 4-4"></path><path d="m3 17 2 2 4-4"></path><line x1="13" y1="6" x2="21" y2="6"></line><line x1="13" y1="12" x2="21" y2="12"></line><line x1="13" y1="18" x2="21" y2="18"></line></svg>`;
    case "report":
      return `<svg ${s}><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline><line x1="8" y1="18" x2="8" y2="15"></line><line x1="12" y1="18" x2="12" y2="12"></line><line x1="16" y1="18" x2="16" y2="14"></line></svg>`;
    default:
      return `<svg ${s}><circle cx="6" cy="6" r="3"></circle><circle cx="18" cy="6" r="3"></circle><circle cx="12" cy="18" r="3"></circle><path d="M8.5 7.5 11 15.5"></path><path d="M15.5 7.5 13 15.5"></path><line x1="9" y1="6" x2="15" y2="6"></line></svg>`;
  }
}

function loadCachedWorkflowTemplates() {
  try {
    const raw = localStorage.getItem(WORKFLOW_CACHE_KEY);
    if (raw) {
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed)) {
        return parsed;
      }
    }
  } catch (_) {}
  return DEFAULT_WORKFLOW_TEMPLATES.map((t) => ({ ...t }));
}

function saveCachedWorkflowTemplates(list) {
  try {
    localStorage.setItem(WORKFLOW_CACHE_KEY, JSON.stringify(list));
  } catch (_) {}
}

async function fetchWorkflowTemplatesFromGateway() {
  try {
    const resp = await fetch("/gateway/workflow-templates");
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
    const data = await resp.json();
    if (Array.isArray(data.templates)) {
      workflowState.templates = data.templates;
      workflowState.error = "";
      saveCachedWorkflowTemplates(data.templates);
      renderWorkflowSheet();
    }
  } catch (err) {
    if (!workflowState.templates.length) {
      workflowState.templates = loadCachedWorkflowTemplates();
    }
    console.warn("[Workflows] Gateway fetch failed, using cached templates:", err);
  }
}

async function syncWorkflowTemplatesToGateway(nextTemplates) {
  workflowState.templates = nextTemplates;
  saveCachedWorkflowTemplates(nextTemplates);
  workflowState.syncing = true;
  workflowState.error = "";
  renderWorkflowSheet();

  try {
    const resp = await fetch("/gateway/workflow-templates", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ templates: nextTemplates })
    });
    const data = await resp.json().catch(() => ({}));
    if (!resp.ok) {
      throw new Error(data.error || `HTTP ${resp.status}`);
    }
    if (Array.isArray(data.templates)) {
      workflowState.templates = data.templates;
      saveCachedWorkflowTemplates(data.templates);
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
    const data = await resp.json().catch(() => ({}));
    if (!resp.ok) {
      throw new Error(data.error || `HTTP ${resp.status}`);
    }
    const list = Array.isArray(data.templates) ? data.templates : DEFAULT_WORKFLOW_TEMPLATES.map((t) => ({ ...t }));
    workflowState.templates = list;
    saveCachedWorkflowTemplates(list);
  } catch (err) {
    workflowState.templates = DEFAULT_WORKFLOW_TEMPLATES.map((t) => ({ ...t }));
    saveCachedWorkflowTemplates(workflowState.templates);
    workflowState.error = "已重置本机列表，网关同步失败：" + (err.message || err);
  } finally {
    workflowState.syncing = false;
    renderWorkflowSheet();
  }
}

function generateWorkflowTemplateId() {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return "wf_" + crypto.randomUUID();
  }
  return "wf_" + Date.now() + "_" + Math.random().toString(36).slice(2, 8);
}

function openWorkflowSheet() {
  const sheet = document.getElementById("sheet-workflows");
  if (!sheet) return;
  if (typeof triggerHaptic === "function") triggerHaptic("light");

  workflowState.editing = false;
  workflowState.formMode = null;
  workflowState.error = "";
  workflowState.confirmReset = false;
  workflowState.templates = loadCachedWorkflowTemplates();

  renderWorkflowSheet();
  sheet.classList.remove("hidden");
  fetchWorkflowTemplatesFromGateway();
}

function closeWorkflowSheet() {
  const sheet = document.getElementById("sheet-workflows");
  if (!sheet) return;
  workflowState.editing = false;
  workflowState.formMode = null;
  workflowState.confirmReset = false;
  sheet.classList.add("hidden");
}

function applyWorkflowTemplate(promptText) {
  const input = document.getElementById("chat-input");
  if (input) {
    input.value = promptText || "";
    input.style.height = "auto";
    if (input.value) {
      input.style.height = Math.min(input.scrollHeight, 120) + "px";
    }
    input.dispatchEvent(new Event("input", { bubbles: true }));
    input.focus();
  }
  if (typeof triggerHaptic === "function") triggerHaptic("medium");
  closeWorkflowSheet();
}

function openWorkflowForm(mode, item) {
  workflowState.formMode = {
    mode,
    item: item
      ? { ...item }
      : {
          id: generateWorkflowTemplateId(),
          title: "",
          subtitle: "",
          icon: "workflow",
          color: "indigo",
          templatePrompt: ""
        }
  };
  workflowState.error = "";
  renderWorkflowSheet();
}

function deleteWorkflowTemplate(id) {
  const next = workflowState.templates.filter((t) => t.id !== id);
  if (typeof triggerHaptic === "function") triggerHaptic("medium");
  syncWorkflowTemplatesToGateway(next);
}

function renderWorkflowHeader() {
  const leftBtn = document.getElementById("btn-workflows-left");
  const titleEl = document.getElementById("workflows-title");
  const rightBtn = document.getElementById("btn-workflows-right");
  if (!leftBtn || !titleEl || !rightBtn) return;

  const pencilSvg = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round"><path d="M12 20h9"></path><path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z"></path></svg>`;
  const checkSvg = `<svg width="19" height="19" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"></polyline></svg>`;
  const backSvg = `<svg width="19" height="19" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"></polyline></svg>`;
  const plusSvg = `<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.3" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"></line><line x1="5" y1="12" x2="19" y2="12"></line></svg>`;

  if (workflowState.formMode) {
    leftBtn.innerHTML = backSvg;
    leftBtn.setAttribute("aria-label", "返回");
    leftBtn.classList.remove("primary");
    titleEl.textContent = workflowState.formMode.mode === "add" ? "新增 Workflow" : "编辑 Workflow";
    rightBtn.textContent = "保存";
    rightBtn.setAttribute("aria-label", "保存");
    rightBtn.classList.add("primary");
  } else {
    leftBtn.innerHTML = workflowState.editing ? checkSvg : pencilSvg;
    leftBtn.setAttribute("aria-label", workflowState.editing ? "完成" : "编辑");
    leftBtn.classList.toggle("primary", workflowState.editing);
    titleEl.textContent = "Workflow 模板";
    rightBtn.innerHTML = plusSvg;
    rightBtn.setAttribute("aria-label", "新增 Workflow 模板");
    rightBtn.classList.add("primary");
  }
}

function renderWorkflowForm(body) {
  const draft = workflowState.formMode.item;
  const iconButtons = WORKFLOW_ICON_CATALOG.map((ic) => {
    const active = draft.icon === ic.key ? "active" : "";
    const colorHex = workflowColorHex(draft.color);
    return `<button type="button" class="wf-icon-option ${active}" data-wf-icon="${escapeHtml(ic.key)}" title="${escapeHtml(ic.label)}" style="${active ? `border-color:${colorHex};color:${colorHex};background:${colorHex}18;` : ""}">
      ${workflowIconSvg(ic.key, 17)}
    </button>`;
  }).join("");

  const colorDots = WORKFLOW_COLOR_CATALOG.map((c) => {
    const active = draft.color === c.key ? "active" : "";
    return `<button type="button" class="wf-color-dot ${active}" data-wf-color="${escapeHtml(c.key)}" title="${escapeHtml(c.label)}" style="background:${c.hex};"></button>`;
  }).join("");

  const errHtml = workflowState.error
    ? `<div class="wf-error-banner">${escapeHtml(workflowState.error)}</div>`
    : "";

  body.innerHTML = `
    <div class="wf-form-container">
      ${errHtml}
      <div class="wf-form-field">
        <label class="wf-form-label" for="wf-input-title">标题 <span class="wf-required">*</span></label>
        <input id="wf-input-title" class="wf-form-input" type="text" maxlength="60" placeholder="例如：每日代码巡检、并发代码审查" value="${escapeHtml(draft.title)}" />
      </div>
      <div class="wf-form-field">
        <label class="wf-form-label" for="wf-input-subtitle">简介（可选）</label>
        <input id="wf-input-subtitle" class="wf-form-input" type="text" maxlength="140" placeholder="简短描述用途，方便快速辨识" value="${escapeHtml(draft.subtitle || "")}" />
      </div>
      <div class="wf-form-field">
        <label class="wf-form-label">图标与主题色</label>
        <div class="wf-color-row">${colorDots}</div>
        <div class="wf-icon-grid">${iconButtons}</div>
      </div>
      <div class="wf-form-field">
        <label class="wf-form-label" for="wf-input-prompt">提示词 / 命令模板 <span class="wf-required">*</span></label>
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
  body.querySelector("#wf-input-prompt")?.addEventListener("input", (e) => {
    workflowState.formMode.item.templatePrompt = e.target.value;
  });
  body.querySelectorAll("[data-wf-icon]").forEach((btn) => {
    btn.addEventListener("click", () => {
      workflowState.formMode.item.icon = btn.dataset.wfIcon;
      renderWorkflowSheet();
    });
  });
  body.querySelectorAll("[data-wf-color]").forEach((btn) => {
    btn.addEventListener("click", () => {
      workflowState.formMode.item.color = btn.dataset.wfColor;
      renderWorkflowSheet();
    });
  });
}

function submitWorkflowForm() {
  if (!workflowState.formMode) return;
  const item = workflowState.formMode.item;
  const title = String(item.title || "").trim();
  const prompt = String(item.templatePrompt || "").trim();
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

  const normalized = {
    id: item.id || generateWorkflowTemplateId(),
    title,
    subtitle: String(item.subtitle || "").trim(),
    icon: item.icon || "workflow",
    color: item.color || "indigo",
    templatePrompt: prompt
  };

  let next;
  if (workflowState.formMode.mode === "add") {
    next = [...workflowState.templates, normalized];
  } else {
    next = workflowState.templates.map((t) => (t.id === normalized.id ? normalized : t));
  }
  workflowState.formMode = null;
  syncWorkflowTemplatesToGateway(next);
}

function bindWorkflowDragReorder(listEl) {
  if (!workflowState.editing || !listEl) return;

  listEl.querySelectorAll(".wf-drag-handle").forEach((handle) => {
    handle.addEventListener("pointerdown", (startEvt) => {
      startEvt.preventDefault();
      startEvt.stopPropagation();
      const wrapper = handle.closest(".wf-card-wrapper");
      if (!wrapper) return;

      const startIndex = Number(wrapper.dataset.index);
      const wrappers = Array.from(listEl.querySelectorAll(".wf-card-wrapper"));
      const rects = wrappers.map((el) => el.getBoundingClientRect());
      let targetIndex = startIndex;

      wrapper.classList.add("dragging");
      handle.setPointerCapture(startEvt.pointerId);

      const onMove = (moveEvt) => {
        const y = moveEvt.clientY;
        const dy = y - startEvt.clientY;
        wrapper.style.transform = `translateY(${dy}px)`;

        let nextTarget = startIndex;
        for (let i = 0; i < rects.length; i++) {
          const mid = rects[i].top + rects[i].height / 2;
          if (i < startIndex && y < mid) {
            nextTarget = i;
            break;
          }
          if (i > startIndex && y > mid) {
            nextTarget = i;
          }
        }
        if (nextTarget !== targetIndex) {
          targetIndex = nextTarget;
          wrappers.forEach((el, idx) => {
            el.classList.remove("drop-above", "drop-below");
            if (idx === targetIndex && idx !== startIndex) {
              el.classList.add(targetIndex < startIndex ? "drop-above" : "drop-below");
            }
          });
        }
      };

      const finish = () => {
        wrapper.classList.remove("dragging");
        wrapper.style.transform = "";
        wrappers.forEach((el) => el.classList.remove("drop-above", "drop-below"));
        handle.removeEventListener("pointermove", onMove);
        handle.removeEventListener("pointerup", finish);
        handle.removeEventListener("pointercancel", finish);

        if (targetIndex !== startIndex) {
          const next = workflowState.templates.slice();
          const [moved] = next.splice(startIndex, 1);
          next.splice(targetIndex, 0, moved);
          if (typeof triggerHaptic === "function") triggerHaptic("selection");
          syncWorkflowTemplatesToGateway(next);
        }
      };

      handle.addEventListener("pointermove", onMove);
      handle.addEventListener("pointerup", finish);
      handle.addEventListener("pointercancel", finish);
    });
  });
}

function closeAllSwipedWorkflowCards(listEl, exceptWrapper) {
  if (!listEl) return;
  listEl.querySelectorAll(".wf-card-wrapper.swiped").forEach((w) => {
    if (w !== exceptWrapper) {
      w.classList.remove("swiped", "swiping");
      const card = w.querySelector(".wf-card");
      if (card) card.style.transform = "";
    }
  });
}

function bindWorkflowSwipeDelete(listEl) {
  if (!listEl) return;
  const maxReveal = 68;

  listEl.querySelectorAll(".wf-card-wrapper").forEach((wrapper) => {
    const card = wrapper.querySelector(".wf-card");
    if (!card) return;

    let startX = 0;
    let startY = 0;
    let currentX = 0;
    let swiping = false;
    let lockedAxis = null;

    card.addEventListener("pointerdown", (e) => {
      if (e.target.closest(".wf-drag-handle") || e.target.closest("[data-wf-edit]")) return;
      startX = e.clientX;
      startY = e.clientY;
      currentX = wrapper.classList.contains("swiped") ? -maxReveal : 0;
      swiping = true;
      lockedAxis = null;
      closeAllSwipedWorkflowCards(listEl, wrapper);
    });

    card.addEventListener("pointermove", (e) => {
      if (!swiping) return;
      const dx = e.clientX - startX;
      const dy = e.clientY - startY;

      if (!lockedAxis) {
        if (Math.abs(dx) < 6 && Math.abs(dy) < 6) return;
        lockedAxis = Math.abs(dx) > Math.abs(dy) ? "x" : "y";
      }
      if (lockedAxis !== "x") {
        swiping = false;
        return;
      }

      wrapper.classList.add("swiping");
      const nextX = Math.max(-maxReveal, Math.min(0, currentX + dx));
      card.style.transform = `translateX(${nextX}px)`;
    });

    const endSwipe = (e) => {
      if (!swiping) return;
      swiping = false;
      wrapper.classList.remove("swiping");
      if (lockedAxis !== "x") return;

      const dx = e.clientX - startX;
      const finalX = Math.max(-maxReveal, Math.min(0, currentX + dx));
      if (finalX < -maxReveal * 0.45) {
        wrapper.classList.add("swiped");
        card.style.transform = `translateX(-${maxReveal}px)`;
      } else {
        wrapper.classList.remove("swiped");
        card.style.transform = "";
      }
    };

    card.addEventListener("pointerup", endSwipe);
    card.addEventListener("pointercancel", endSwipe);
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

  const list = workflowState.templates;
  let cardsHtml = "";
  if (list.length === 0) {
    cardsHtml = `
      <div class="wf-empty-state">
        <p>暂无 Workflow 模板</p>
        <div class="wf-empty-actions">
          <button type="button" class="wf-action-pill primary" id="btn-wf-empty-add">+ 新增模板</button>
          <button type="button" class="wf-action-pill" id="btn-wf-empty-reset">恢复默认</button>
        </div>
      </div>
    `;
  } else {
    cardsHtml = list
      .map((item, idx) => {
        const hex = workflowColorHex(item.color);
        const sub = item.subtitle || item.templatePrompt || "";
        const iconHtml = `
          <div class="wf-card-icon" style="background:${hex}22;color:${hex};">
            ${workflowIconSvg(item.icon, 18)}
          </div>
        `;
        const deleteActionHtml = `
          <div class="wf-card-actions">
            <button class="wf-card-delete-btn" type="button" aria-label="删除" data-wf-del="${escapeHtml(item.id)}">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="3 6 5 6 21 6"></polyline>
                <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path>
              </svg>
            </button>
          </div>
        `;

        if (workflowState.editing) {
          return `
            <div class="wf-card-wrapper" data-id="${escapeHtml(item.id)}" data-index="${idx}">
              ${deleteActionHtml}
              <div class="wf-card editing">
                <button type="button" class="wf-drag-handle" aria-label="拖拽排序" title="按住拖拽排序">
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round">
                    <line x1="5" y1="8" x2="19" y2="8"></line>
                    <line x1="5" y1="12" x2="19" y2="12"></line>
                    <line x1="5" y1="16" x2="19" y2="16"></line>
                  </svg>
                </button>
                ${iconHtml}
                <div class="wf-card-text">
                  <div class="wf-card-title">${escapeHtml(item.title)}</div>
                  <div class="wf-card-subtitle">${escapeHtml(sub)}</div>
                </div>
                <button type="button" class="wf-icon-btn" data-wf-edit="${escapeHtml(item.id)}" aria-label="编辑" title="编辑">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 20h9"></path><path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z"></path></svg>
                </button>
              </div>
            </div>
          `;
        }
        return `
          <div class="wf-card-wrapper" data-id="${escapeHtml(item.id)}" data-index="${idx}">
            ${deleteActionHtml}
            <div class="wf-card" data-wf-pick="${escapeHtml(item.id)}">
              ${iconHtml}
              <div class="wf-card-text">
                <div class="wf-card-title">${escapeHtml(item.title)}</div>
                <div class="wf-card-subtitle">${escapeHtml(sub)}</div>
              </div>
              <svg class="wf-card-chevron" width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="9 18 15 12 9 6"></polyline>
              </svg>
            </div>
          </div>
        `;
      })
      .join("");
  }

  let footerHtml = "";
  if (workflowState.editing) {
    if (workflowState.confirmReset) {
      footerHtml = `
        <div class="wf-reset-confirm">
          <span>确定恢复为系统初始的 4 个模板？（自定义模板将被清除）</span>
          <div class="wf-reset-confirm-btns">
            <button type="button" class="wf-action-pill" id="btn-wf-reset-cancel">取消</button>
            <button type="button" class="wf-action-pill danger" id="btn-wf-reset-confirm">确认恢复</button>
          </div>
        </div>
      `;
    } else {
      footerHtml = `
        <div class="wf-edit-footer">
          <button type="button" class="wf-reset-btn" id="btn-wf-reset">恢复默认模板</button>
        </div>
      `;
    }
  }

  body.innerHTML = `
    <div class="wf-banner">
      <div class="wf-banner-icon">${workflowIconSvg("workflow", 18)}</div>
      <div class="wf-banner-text">
        <div class="wf-banner-title">Antigravity 动态工作流</div>
        <div class="wf-banner-sub">${workflowState.editing ? "按住最左侧手柄拖拽排序，左滑可删除模板" : "选择常用模板快速填充，左滑可删除"}</div>
      </div>
    </div>
    ${errHtml}
    <div id="workflows-list" class="wf-list">${cardsHtml}</div>
    ${footerHtml}
  `;

  const listEl = body.querySelector("#workflows-list");
  bindWorkflowSwipeDelete(listEl);

  body.querySelectorAll("[data-wf-del]").forEach((btn) => {
    btn.addEventListener("click", (e) => {
      e.stopPropagation();
      deleteWorkflowTemplate(btn.dataset.wfDel);
    });
  });

  if (workflowState.editing) {
    bindWorkflowDragReorder(listEl);

    body.querySelectorAll("[data-wf-edit]").forEach((btn) => {
      btn.addEventListener("click", () => {
        const found = workflowState.templates.find((t) => t.id === btn.dataset.wfEdit);
        if (found) openWorkflowForm("edit", found);
      });
    });
    body.querySelector("#btn-wf-reset")?.addEventListener("click", () => {
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
    body.querySelectorAll("[data-wf-pick]").forEach((card) => {
      card.addEventListener("click", () => {
        const wrapper = card.closest(".wf-card-wrapper");
        if (wrapper && wrapper.classList.contains("swiped")) {
          wrapper.classList.remove("swiped");
          card.style.transform = "";
          return;
        }
        const found = workflowState.templates.find((t) => t.id === card.dataset.wfPick);
        if (found) applyWorkflowTemplate(found.templatePrompt);
      });
    });
  }

  body.querySelector("#btn-wf-empty-add")?.addEventListener("click", () => openWorkflowForm("add"));
  body.querySelector("#btn-wf-empty-reset")?.addEventListener("click", () => resetWorkflowTemplatesOnGateway());
}

function initWorkflowTemplates() {
  document.getElementById("btn-workflow-templates")?.addEventListener("click", openWorkflowSheet);

  const sheet = document.getElementById("sheet-workflows");
  sheet?.addEventListener("click", (e) => {
    if (e.target === sheet) closeWorkflowSheet();
  });
  if (typeof enableSheetPullToDismiss === "function") {
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
