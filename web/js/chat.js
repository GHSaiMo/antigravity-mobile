// --- Chat View & Real-Time Stream ---

let activeWs = null;
let wsReconnectTimer = null;
let wsReconnectAttempts = 0;
let draftDebounceTimer = null;
let userIsNearBottom = true;
let isUserTouching = false;
let hasInitiallyAligned = false;
let prevWasRunning = false;
let currentCanProceed = false;
let currentProceedArtifactUri = null;
let pendingRenderRaf = null;
let pendingRenderData = null;

function cleanInternalSystemMessage(text) {
  if (!text) return "";
  let t = String(text).trim();
  if (!t) return "";

  // Strip "The following is a <SYSTEM_MESSAGE>..." preamble first
  t = t.replace(/The following is a <SYSTEM_MESSAGE> not actually sent by the user[\s\S]*?(?:<SYSTEM_MESSAGE>|$)/gi, "");
  // Strip <SYSTEM_MESSAGE>...</SYSTEM_MESSAGE>
  t = t.replace(/<SYSTEM_MESSAGE>[\s\S]*?<\/SYSTEM_MESSAGE>/gi, "");
  t = t.trim();

  if (!t || t === "</SYSTEM_MESSAGE>" || t === "<SYSTEM_MESSAGE>") {
    return "";
  }
  if (t.startsWith("<SYSTEM_MESSAGE>") || t.startsWith("The following is a <SYSTEM_MESSAGE>")) {
    return "";
  }
  if (t.startsWith("[Message]") && t.includes("priority=MESSAGE_PRIORITY_")) {
    return "";
  }
  if (t.startsWith('Task id "') && t.includes("finished with result:")) {
    return "";
  }
  return t;
}

function updateProceedButton(canProceed) {
  const proceedBtn = document.getElementById("btn-proceed");
  if (proceedBtn) {
    if (canProceed) {
      proceedBtn.classList.remove("hidden");
    } else {
      proceedBtn.classList.add("hidden");
    }
  }
}

function updateContinueButton(canContinue) {
  const continueBtn = document.getElementById("btn-continue");
  if (continueBtn) {
    if (canContinue) {
      continueBtn.classList.remove("hidden");
    } else {
      continueBtn.classList.add("hidden");
    }
  }
}

function checkLatestMessageIsError(stepsOrItems, isRunning, preGrouped = false) {
  if (isRunning) return false;
  if (!stepsOrItems || stepsOrItems.length === 0) return false;
  const items = preGrouped ? stepsOrItems : groupSteps(stepsOrItems);
  if (items.length === 0) return false;
  const lastItem = items[items.length - 1];
  if (lastItem && lastItem.type === "error") return true;
  for (let i = items.length - 1; i >= 0; i--) {
    if (items[i].type !== "user") {
      return items[i].type === "error";
    }
  }
  return false;
}

// --- Floating Interaction Card Management ---
let currentPendingInteraction = null;
let selectedInteractionOptionId = null;
let isSubmittingInteraction = false;
let isInteractionCardExpanded = false;
let isInteractionDiffExpanded = false;

function updatePendingInteraction(interaction, isRunning) {
  const container = document.getElementById("interaction-card-container");
  if (!container) return;

  const hasOptionsOrQuestions = (interaction?.options && interaction.options.length > 0) ||
    interaction?.hasWriteIn ||
    (interaction?.questions && interaction.questions.length > 0);
  if (!isRunning || !interaction || !hasOptionsOrQuestions) {
    currentPendingInteraction = null;
    selectedInteractionOptionId = null;
    singleMultiSelections = [];
    singleWriteInActive = false;
    singleWriteInText = "";
    multiQuestionSelections = {};
    multiQuestionMultiSelections = {};
    multiQuestionWriteInActives = {};
    multiQuestionWriteIns = {};
    isInteractionCardExpanded = false;
    isInteractionDiffExpanded = false;
    container.innerHTML = "";
    container.classList.add("hidden");
    return;
  }

  const isDifferent = !currentPendingInteraction ||
    currentPendingInteraction.stepIndex !== interaction.stepIndex ||
    currentPendingInteraction.type !== interaction.type ||
    currentPendingInteraction.title !== interaction.title;

  currentPendingInteraction = interaction;
  if (isDifferent) {
    selectedInteractionOptionId = interaction.defaultOptionId || interaction.options?.[0]?.id || (interaction.hasWriteIn ? "__write_in__" : "");
    singleMultiSelections = [];
    singleWriteInActive = false;
    singleWriteInText = "";
    multiQuestionSelections = {};
    multiQuestionMultiSelections = {};
    multiQuestionWriteInActives = {};
    multiQuestionWriteIns = {};
    isInteractionCardExpanded = false;
    isInteractionDiffExpanded = false;
  } else if (!selectedInteractionOptionId) {
    selectedInteractionOptionId = interaction.defaultOptionId || interaction.options?.[0]?.id || (interaction.hasWriteIn ? "__write_in__" : "");
  }

  renderInteractionCard(isDifferent);
}

let singleMultiSelections = [];
let singleWriteInActive = false;
let multiQuestionSelections = {};
let multiQuestionMultiSelections = {};
let multiQuestionWriteInActives = {};
let multiQuestionWriteIns = {};
let singleWriteInText = "";

function isInteractionOptionWriteIn(opt, hasWriteIn, isPermission) {
  if (!opt) return false;
  if (opt.id === "__write_in__") return true;
  if (!hasWriteIn) return false;
  if (opt.isDeny) return true;
  if (isPermission && opt.id === "5") return true;
  return false;
}

function buildOptionIndicatorHtml(isMulti, isSelected, extraClass = "") {
  if (isMulti) {
    return `<span class="interaction-option-checkbox ${extraClass}" style="margin-top: 1.5px; width: 16px; height: 16px; border-radius: 4px; border: 1.5px solid ${isSelected ? '#3b82f6' : 'var(--text-secondary)'}; background: ${isSelected ? '#3b82f6' : 'transparent'}; display: flex; align-items: center; justify-content: center; flex-shrink: 0;">${isSelected ? '<svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="#ffffff" stroke-width="3.2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"></polyline></svg>' : ''}</span>`;
  }
  return `<span class="interaction-option-radio" style="margin-top: 2px; width: 15px; height: 15px; border-radius: 50%; border: 1.5px solid ${isSelected ? '#3b82f6' : 'var(--text-secondary)'}; display: flex; align-items: center; justify-content: center; flex-shrink: 0;">${isSelected ? '<span style="width: 7px; height: 7px; border-radius: 50%; background: #3b82f6;"></span>' : ''}</span>`;
}

function resolveWriteInLabel(raw) {
  if (!raw || !raw.trim()) return "自定义输入";
  const lower = raw.trim().toLowerCase();
  if (lower === "other" || lower === "write in" || lower === "write in...") return "自定义输入";
  return raw;
}

function resolveWriteInPlaceholder(raw, isDeny) {
  if (!raw || !raw.trim()) return isDeny ? "告诉 Agent 应该怎么做..." : "输入自定义回答...";
  const lower = raw.trim().toLowerCase();
  if (lower.includes("write in your response") || lower.includes("write in")) return "输入自定义回答...";
  if (lower.includes("tell the agent") || lower.includes("what to do instead")) return "告诉 Agent 应该怎么做...";
  return raw;
}

function resolveCompactInteractionCategory(interaction) {
  if (interaction.permissionGrant) return "权限确认";
  switch (interaction.type) {
    case "permission":
    case "file_permission":
      return "权限确认";
    case "browser_download":
      return "下载确认";
    case "approval":
      return "操作确认";
    case "ask_question":
      return "询问用户";
    case "run_command":
      return "执行命令";
    case "mcp":
      return "MCP 工具确认";
    case "elicitation":
      return "补充信息";
    default:
      return "需要确认";
  }
}

function resolveApprovalRiskReason(code) {
  const tr = (typeof window !== "undefined" && typeof window.t === "function") ? window.t : (s) => s;
  switch (code) {
    case "destructive_delete": return tr("破坏性删除");
    case "force_git": return tr("Git 强推/硬重置");
    case "privilege_exec": return tr("提权执行");
    case "remote_pipe_exec": return tr("远程脚本执行");
    case "destructive_sql": return tr("高危 SQL 删除");
    case "system_ops": return tr("系统级操作");
    case "sensitive_path": return tr("敏感凭证路径");
    case "outside_workspace": return tr("工作区外路径");
    case "overwrite_file": return tr("覆盖写入");
    case "external_write": return tr("外部写入操作");
    case "git_push": return tr("推送远端");
    default: return code;
  }
}

function buildApprovalRiskSummary(preview) {
  if (!preview) return null;
  const tr = (typeof window !== "undefined" && typeof window.t === "function") ? window.t : (s) => s;
  const reasons = Array.isArray(preview.riskReasons) ? preview.riskReasons.map(resolveApprovalRiskReason) : [];
  if (preview.riskLevel === "high") {
    const prefix = tr("高危操作");
    return { level: "high", text: reasons.length > 0 ? `${prefix} · ${reasons.join(" · ")}` : prefix };
  }
  if (preview.riskLevel === "medium" && reasons.length > 0) {
    const prefix = tr("注意");
    return { level: "medium", text: `${prefix} · ${reasons.join(" · ")}` };
  }
  return null;
}

function buildApprovalDiffHtml(preview) {
  if (!preview || !preview.diffPreview) return "";
  const tr = (typeof window !== "undefined" && typeof window.t === "function") ? window.t : (s) => s;
  const lines = String(preview.diffPreview).split("\n");
  if (lines.length === 0) return "";
  const added = Number(preview.addedLines || 0);
  const removed = Number(preview.removedLines || 0);
  const isNew = Boolean(preview.isNewFile) && removed === 0;
  const truncated = Boolean(preview.diffTruncated);
  const toggleLabel = isInteractionDiffExpanded ? tr("收起改动") : tr("预览改动");
  const diffLinesHtml = isInteractionDiffExpanded ? `
    <div class="interaction-diff-box">
      ${lines.map(line => {
        const cls = line.startsWith("+ ") ? "is-add" : (line.startsWith("- ") ? "is-del" : (line === "@@" ? "is-hunk" : ""));
        return `<div class="interaction-diff-line ${cls}">${escapeHtml(line || " ")}</div>`;
      }).join("")}
      ${truncated ? `<div class="interaction-diff-truncated">${escapeHtml(tr("仅展示前 40 行改动"))}</div>` : ""}
    </div>
  ` : "";

  return `
    <div class="interaction-diff-wrap">
      <button type="button" id="btn-interaction-diff-toggle" class="interaction-diff-toggle">
        <span class="interaction-diff-toggle-main">
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline><line x1="9" y1="15" x2="15" y2="15"></line></svg>
          <span>${escapeHtml(toggleLabel)}</span>
          ${isNew ? `<span class="interaction-diff-new">${escapeHtml(tr("新建"))}</span>` : ""}
          ${added > 0 ? `<span class="interaction-diff-added">+${added}</span>` : ""}
          ${removed > 0 ? `<span class="interaction-diff-removed">-${removed}</span>` : ""}
        </span>
        <span class="interaction-diff-chevron">${isInteractionDiffExpanded ? "▴" : "▾"}</span>
      </button>
      ${diffLinesHtml}
    </div>
  `;
}

function renderInteractionCard(resetScroll = false) {
  const container = document.getElementById("interaction-card-container");
  if (!container || !currentPendingInteraction) return;

  const prevBody = container.querySelector(".interaction-card-body");
  const prevScrollTop = (!resetScroll && prevBody) ? prevBody.scrollTop : 0;

  const interaction = currentPendingInteraction;

  // 动态多智能体工作流安全审批卡片 (Antigravity 2.32+)
  if (interaction.type === "workflow_approval" || interaction.workflowApproval) {
    const wa = interaction.workflowApproval || {};
    const phases = wa.estimatedPhases || [];
    const script = wa.script || interaction.target || "";
    container.innerHTML = `
      <div class="interaction-card workflow-approval-box">
        <div class="interaction-card-header" style="display: flex; align-items: center; gap: 8px;">
          <div class="interaction-icon" style="color: #a855f7;">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2v20M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6"></path></svg>
          </div>
          <div class="interaction-title-group" style="flex: 1;">
            <div class="interaction-title" style="font-size: 14px; font-weight: bold; color: var(--text-primary); display: flex; align-items: center; gap: 6px;">
              <span>Workflow 安全审批</span>
              <span class="badge" style="font-size: 10px; background: rgba(168, 85, 247, 0.2); color: #c084fc;">run_workflow</span>
            </div>
          </div>
        </div>

        <div style="font-size: 13px; color: var(--text-secondary); line-height: 1.4;">
          ${escapeHtml(wa.description || interaction.description || "Agent 请求执行多智能体动态工作流编排脚本。")}
        </div>

        ${phases.length > 0 ? `
          <div style="display: flex; align-items: center; gap: 6px; flex-wrap: wrap;">
            <span style="font-size: 11px; color: var(--text-muted);">预估阶段:</span>
            ${phases.map(p => `<span class="badge" style="font-size: 11px; background: var(--bg-tertiary); color: var(--text-secondary);">${escapeHtml(p)}</span>`).join('')}
          </div>
        ` : ''}

        ${script ? `
          <details style="background: rgba(0,0,0,0.3); border-radius: 8px; padding: 6px 10px; font-size: 12px;">
            <summary style="cursor: pointer; color: #a855f7; font-weight: 500;">查看 Python 编排脚本 (${script.split('\n').length} 行)</summary>
            <pre style="margin-top: 8px; max-height: 160px; overflow-y: auto; font-family: monospace; font-size: 11px; color: #cbd5e1; white-space: pre-wrap;"><code>${escapeHtml(script)}</code></pre>
          </details>
        ` : ''}

        <div id="workflow-feedback-container" class="hidden" style="margin-top: 4px;">
          <input type="text" id="workflow-feedback-input" placeholder="输入改进指示并拒绝..." style="width: 100%; box-sizing: border-box; padding: 8px 10px; border-radius: 6px; border: 1px solid var(--border-color); background: var(--bg-tertiary); color: var(--text-primary); font-size: 12px;" />
        </div>

        <div class="workflow-approval-actions">
          <button type="button" class="workflow-approval-btn allow-once" data-action="once">允许运行本次</button>
          <button type="button" class="workflow-approval-btn allow-session" data-action="session">允许本会话运行</button>
          <button type="button" class="workflow-approval-btn deny" data-action="deny">拒绝</button>
          <button type="button" class="workflow-approval-btn deny-feedback" data-action="feedback">拒绝并指导</button>
        </div>
      </div>
    `;

    container.classList.remove("hidden");

    container.querySelectorAll(".workflow-approval-btn").forEach(btn => {
      btn.addEventListener("click", () => {
        const action = btn.dataset.action;
        if (action === "feedback") {
          const fbContainer = container.querySelector("#workflow-feedback-container");
          const fbInput = container.querySelector("#workflow-feedback-input");
          if (fbContainer.classList.contains("hidden")) {
            fbContainer.classList.remove("hidden");
            fbInput.focus();
            btn.textContent = "确认拒绝并发送";
            btn.classList.remove("deny-feedback");
            btn.classList.add("deny");
          } else {
            const text = fbInput.value.trim();
            handleInteractionSubmitDirect("deny_with_feedback", text);
          }
        } else {
          handleInteractionSubmitDirect(action, null);
        }
      });
    });
    return;
  }

  const isPermission = interaction.type === "permission" || interaction.type === "file_permission";
  const isDownload = interaction.type === "browser_download";
  const isMcp = interaction.type === "mcp";
  const isElicitation = interaction.type === "elicitation";
  const isApproval = interaction.type === "approval" || isDownload || isMcp;
  const hasMultiQuestions = interaction.questions && interaction.questions.length > 1;
  const isSingleMultiSelect = !hasMultiQuestions && Boolean(interaction.isMultiSelect || interaction.questions?.[0]?.isMultiSelect);
  const tr = (typeof window !== "undefined" && typeof window.t === "function") ? window.t : (s) => s;
  const multiSelectBadgeHtml = `<span class="interaction-multiselect-badge" style="display: inline-flex; align-items: center; padding: 1px 7px; border-radius: 999px; background: rgba(59,130,246,0.14); color: #3b82f6; font-size: 11px; font-weight: 600; line-height: 1.4; white-space: nowrap; flex-shrink: 0;">${escapeHtml(tr("多选"))}</span>`;
  const preview = interaction.approvalPreview || null;
  const isHighRisk = preview && preview.riskLevel === "high";
  const riskSummary = buildApprovalRiskSummary(preview);
  const diffHtml = buildApprovalDiffHtml(preview);

  const singleMultiCount = isSingleMultiSelect
    ? singleMultiSelections.length + (singleWriteInActive && singleWriteInText.trim().length > 0 ? 1 : 0)
    : 0;
  const canSubmit = hasMultiQuestions
    ? interaction.questions.every((q, idx) => {
        if (!q.isMultiSelect) return true;
        const selected = multiQuestionMultiSelections[idx] || [];
        const hasWriteIn = Boolean(multiQuestionWriteInActives[idx]) && Boolean((multiQuestionWriteIns[idx] || "").trim());
        return selected.length > 0 || hasWriteIn;
      })
    : (!isSingleMultiSelect || singleMultiCount > 0);
  const submitLabelText = isSubmittingInteraction
    ? tr("提交中...")
    : (isSingleMultiSelect && singleMultiCount > 0 ? `${tr("提交")} (${singleMultiCount})` : tr("提交"));

  let iconSvg = '';
  if (isHighRisk) {
    iconSvg = `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="#ef4444" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"></path><line x1="12" y1="9" x2="12" y2="13"></line><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>`;
  } else if (isDownload) {
    iconSvg = `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="#f59e0b" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path><polyline points="7 10 12 15 17 10"></polyline><line x1="12" y1="15" x2="12" y2="3"></line></svg>`;
  } else if (isMcp) {
    iconSvg = `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="#8b5cf6" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22v-5"></path><path d="M9 8V2"></path><path d="M15 8V2"></path><path d="M18 8v5a4 4 0 0 1-4 4h-4a4 4 0 0 1-4-4V8Z"></path></svg>`;
  } else if (isElicitation) {
    iconSvg = `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="#06b6d4" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"></path><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"></path></svg>`;
  } else if (isPermission) {
    iconSvg = `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"></path></svg>`;
  } else {
    iconSvg = `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"></path><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>`;
  }

  const singleTitle = interaction.title || interaction.description || "需要确认或授权";
  const isLongSingleQuestion = !hasMultiQuestions && (singleTitle.includes("\n") || singleTitle.length > 48);
  const headerTitleText = hasMultiQuestions
    ? `需要确认规格 (${interaction.questions.length} 个问题)`
    : (isLongSingleQuestion ? resolveCompactInteractionCategory(interaction) : singleTitle);

  let contentHtml = '';
  if (hasMultiQuestions) {
    contentHtml = `
      <div class="interaction-multi-questions-list" style="display: flex; flex-direction: column; gap: 12px;">
        ${interaction.questions.map((q, qIdx) => {
          const isQMulti = Boolean(q.isMultiSelect);
          const hasWriteIn = q.hasWriteIn && !(q.options || []).some(o => o.id === '__write_in__' || o.isDeny);
          const writeInLabel = resolveWriteInLabel(q.writeInLabel);
          const opts = hasWriteIn
            ? [...(q.options || []), { id: '__write_in__', text: writeInLabel }]
            : (q.options || []);
          const selectedId = multiQuestionSelections[qIdx] || q.defaultOptionId || opts[0]?.id || "1";
          const selectedSet = multiQuestionMultiSelections[qIdx] || [];
          const qWriteInActive = Boolean(multiQuestionWriteInActives[qIdx]);
          const isNumeric = (q.options || []).length > 0 && (q.options || []).every(o => !isNaN(parseInt(o.id, 10)));
          return `
            <div class="interaction-question-block" style="border: 1px solid var(--border-color, rgba(255,255,255,0.08)); border-radius: 8px; padding: 10px; background: rgba(0,0,0,0.15);">
              <div style="display: flex; align-items: flex-start; gap: 6px; margin-bottom: 8px;">
                <span style="background: #9333ea; color: white; font-size: 10px; font-weight: bold; padding: 2px 5px; border-radius: 4px; font-family: monospace; flex-shrink: 0;">Q${qIdx + 1}</span>
                <span style="font-size: 13px; font-weight: 600; color: var(--text-primary); line-height: 1.4; white-space: pre-wrap; word-break: break-word; flex: 1; min-width: 0;">${escapeHtml(q.question)}</span>
                ${isQMulti ? multiSelectBadgeHtml : ''}
              </div>
              <div class="interaction-options-list" style="display: flex; flex-direction: column; gap: 5px;">
                ${opts.map((opt, optIdx) => {
                  const isWriteIn = isInteractionOptionWriteIn(opt, q.hasWriteIn, false);
                  const isSelected = isQMulti
                    ? (isWriteIn ? qWriteInActive : selectedSet.includes(opt.id))
                    : (opt.id === selectedId);
                  const label = opt.text || opt.label || "";
                  const badge = opt.id === '__write_in__' ? (isNumeric ? String(optIdx + 1) : '✎') : (opt.id || String(optIdx + 1));
                  const toggleCls = (isQMulti && isWriteIn && isSelected) ? 'js-toggle-writein-off' : '';
                  return `
                    <div class="interaction-option-item ${isSelected ? 'selected' : ''}" data-q-idx="${qIdx}" data-opt-id="${escapeHtml(opt.id)}" data-is-multi="${isQMulti ? '1' : '0'}" data-is-writein="${isWriteIn ? '1' : '0'}" style="display: flex; align-items: flex-start; gap: 8px; padding: 7px 9px; border-radius: 6px; cursor: pointer; background: ${isSelected ? 'rgba(59,130,246,0.12)' : 'var(--bg-tertiary, rgba(255,255,255,0.04))'}; border: 1px solid ${isSelected ? '#3b82f6' : 'transparent'};">
                      <span class="interaction-option-badge ${toggleCls}" style="margin-top: 1px; font-size: 11px; font-weight: bold; font-family: monospace; color: ${isSelected ? '#3b82f6' : 'var(--text-secondary)'}; flex-shrink: 0;">[${escapeHtml(badge)}]</span>
                      ${isWriteIn && isSelected ? `
                        <input type="text" class="interaction-multi-write-in-input" data-q-idx="${qIdx}" placeholder="${escapeHtml(resolveWriteInPlaceholder(q.writeInPlaceholder, false))}" value="${escapeHtml(multiQuestionWriteIns[qIdx] || '')}" style="flex: 1; font-size: 12.5px; border: none; background: transparent; color: var(--text-primary); outline: none; padding: 0;" />
                      ` : `
                        <span class="interaction-option-label" style="font-size: 12.5px; line-height: 1.4; color: ${isSelected ? 'var(--text-primary)' : 'var(--text-secondary)'}; white-space: normal; word-break: break-word; flex: 1;">${escapeHtml(isWriteIn && multiQuestionWriteIns[qIdx] ? multiQuestionWriteIns[qIdx] : label)}</span>
                      `}
                      ${buildOptionIndicatorHtml(isQMulti, isSelected, toggleCls)}
                    </div>
                  `;
                }).join('')}
              </div>
            </div>
          `;
        }).join('')}
      </div>
    `;
  } else {
    const hasWriteIn = interaction.hasWriteIn && !(interaction.options || []).some(o => o.id === '__write_in__' || o.isDeny);
    const writeInLabel = resolveWriteInLabel(interaction.writeInLabel);
    const displayOptions = hasWriteIn
      ? [...(interaction.options || []), { id: '__write_in__', text: writeInLabel }]
      : (interaction.options || []);
    const selectedOpt = displayOptions.find(o => o.id === selectedInteractionOptionId) || displayOptions[0];
    const isDenyOrWriteIn = selectedOpt?.isDeny || (isPermission && selectedInteractionOptionId === "5") || selectedInteractionOptionId === "__write_in__" || selectedOpt?.id === "no" || selectedOpt?.id?.toLowerCase().includes("deny");
    const isNumeric = (interaction.options || []).length > 0 && (interaction.options || []).every(o => !isNaN(parseInt(o.id, 10)));

    contentHtml = `
      <div class="interaction-options-list" style="display: flex; flex-direction: column; gap: 6px;">
        ${displayOptions.map((opt, idx) => {
          const isWriteIn = isInteractionOptionWriteIn(opt, interaction.hasWriteIn, isPermission);
          const isSelected = isSingleMultiSelect
            ? (isWriteIn ? singleWriteInActive : singleMultiSelections.includes(opt.id))
            : (opt.id === selectedInteractionOptionId);
          const label = opt.text || opt.label || "";
          const badge = opt.id === '__write_in__' ? (isNumeric ? String(idx + 1) : '✎') : (opt.id || String(idx + 1));
          const toggleCls = (isSingleMultiSelect && isWriteIn && isSelected) ? 'js-toggle-writein-off' : '';
          return `
            <div class="interaction-option-item ${isSelected ? 'selected' : ''}" data-opt-id="${escapeHtml(opt.id)}" data-is-multi="${isSingleMultiSelect ? '1' : '0'}" data-is-writein="${isWriteIn ? '1' : '0'}" style="display: flex; align-items: flex-start; gap: 8px; padding: 8px 10px; border-radius: 8px; cursor: pointer; background: ${isSelected ? 'rgba(59,130,246,0.12)' : 'var(--bg-tertiary, rgba(255,255,255,0.04))'}; border: 1px solid ${isSelected ? '#3b82f6' : 'transparent'};">
              <span class="interaction-option-badge ${toggleCls}" style="margin-top: 1px; font-size: 11px; font-weight: bold; font-family: monospace; color: ${isSelected ? '#3b82f6' : 'var(--text-secondary)'}; flex-shrink: 0;">[${escapeHtml(badge)}]</span>
              ${isWriteIn && isSelected ? `
                <input type="text" id="interaction-write-in-input" class="interaction-write-in-input" placeholder="${escapeHtml(resolveWriteInPlaceholder(interaction.writeInPlaceholder, isDenyOrWriteIn))}" value="${escapeHtml(singleWriteInText)}" style="flex: 1; font-size: 13px; border: none; background: transparent; color: var(--text-primary); outline: none; padding: 0;" />
              ` : `
                <span class="interaction-option-label" style="font-size: 13px; line-height: 1.45; color: ${isSelected ? 'var(--text-primary)' : 'var(--text-secondary)'}; white-space: normal; word-break: break-word; flex: 1;">${escapeHtml(isWriteIn && singleWriteInText ? singleWriteInText : label)}</span>
              `}
              ${buildOptionIndicatorHtml(isSingleMultiSelect, isSelected, toggleCls)}
            </div>
          `;
        }).join('')}
      </div>
    `;
  }

  const displayTarget = (preview && preview.displayPath && interaction.type !== "run_command")
    ? preview.displayPath
    : (interaction.target || "");
  const cwdText = (preview && preview.cwd) ? preview.cwd : "";
  const riskBannerHtml = riskSummary ? `
    <div class="interaction-risk-banner ${riskSummary.level === "high" ? "is-high" : "is-medium"}">
      <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" style="flex-shrink:0;"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"></path><line x1="12" y1="9" x2="12" y2="13"></line><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>
      <span>${escapeHtml(riskSummary.text)}</span>
    </div>
  ` : "";
  const formatBoldTargetHtml = (raw) => {
    const s = String(raw || "");
    if (!s) return "";
    if (!s.includes("\n")) {
      const slash = s.lastIndexOf("/");
      if (slash >= 0 && slash < s.length - 1) {
        return `${escapeHtml(s.slice(0, slash + 1))}<strong style="color:var(--text-primary);font-weight:700;">${escapeHtml(s.slice(slash + 1))}</strong>`;
      }
    }
    return `<strong style="color:var(--text-primary);font-weight:600;">${escapeHtml(s)}</strong>`;
  };
  const targetBoxHtml = (displayTarget || cwdText) ? `
    <div class="interaction-target-box" style="padding: 6px 10px; border-radius: 6px; background: rgba(0,0,0,0.2); font-family: monospace; font-size: 11.5px; color: ${isHighRisk ? 'var(--text-primary)' : 'var(--text-secondary)'}; word-break: break-all; white-space: pre-wrap;">
      ${displayTarget ? `<div>${formatBoldTargetHtml(displayTarget)}</div>` : ""}
      ${cwdText ? `<div class="interaction-target-cwd">📁 ${escapeHtml(cwdText)}</div>` : ""}
    </div>
  ` : "";

  const cardMaxHeight = isInteractionCardExpanded ? 'min(76dvh, 660px)' : 'min(54dvh, 420px)';
  const cardBorderColor = isHighRisk ? 'rgba(239,68,68,0.48)' : 'rgba(59,130,246,0.35)';
  container.innerHTML = `
    <div class="interaction-card" style="display: flex; flex-direction: column; max-height: ${cardMaxHeight}; padding: 14px; border-radius: 12px; border: 1px solid ${cardBorderColor}; background: var(--bg-secondary, #1a1d24); box-shadow: 0 4px 16px rgba(0,0,0,0.25);">
      <div class="interaction-card-header" style="display: flex; align-items: ${isLongSingleQuestion || hasMultiQuestions ? 'center' : 'flex-start'}; gap: 8px; flex-shrink: 0; padding-bottom: ${isLongSingleQuestion ? '8px' : '6px'}; ${isLongSingleQuestion ? 'border-bottom: 1px solid var(--border-color, rgba(255,255,255,0.08));' : ''}">
        <div class="interaction-icon" style="color: ${isHighRisk ? '#ef4444' : (hasMultiQuestions ? '#9333ea' : '#3b82f6')}; flex-shrink: 0; margin-top: ${isLongSingleQuestion || hasMultiQuestions ? '0' : '2px'};">${iconSvg}</div>
        <div class="interaction-title-group" style="flex: 1; min-width: 0; display: flex; flex-wrap: wrap; align-items: center; gap: 4px 6px;">
          <div class="interaction-title" style="font-size: 14px; font-weight: bold; color: var(--text-primary); line-height: 1.35; ${isLongSingleQuestion || hasMultiQuestions ? 'white-space: nowrap; overflow: hidden; text-overflow: ellipsis;' : 'display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; word-break: break-word;'}">
            ${escapeHtml(headerTitleText)}
          </div>
          ${isSingleMultiSelect ? multiSelectBadgeHtml : ''}
        </div>
        <button type="button" id="btn-interaction-expand" class="btn-interaction-expand" style="display: ${isInteractionCardExpanded ? 'inline-flex' : 'none'}; align-items: center; gap: 3px; padding: 3px 9px; border-radius: 999px; border: none; background: var(--bg-tertiary, rgba(255,255,255,0.08)); color: var(--text-secondary); font-size: 11.5px; cursor: pointer; flex-shrink: 0;">
          <span>${isInteractionCardExpanded ? '收起' : '展开'}</span>
          <span>${isInteractionCardExpanded ? '▾' : '▴'}</span>
        </button>
      </div>

      <div class="interaction-card-body" style="flex: 1 1 auto; min-height: 0; overflow-y: auto; -webkit-overflow-scrolling: touch; overscroll-behavior-y: contain; display: flex; flex-direction: column; gap: 10px; padding: 6px 2px 4px 0;">
        ${isLongSingleQuestion ? `
          <div class="interaction-long-question" style="font-size: 13.5px; font-weight: 500; line-height: 1.5; color: var(--text-primary); white-space: pre-wrap; word-break: break-word;">${escapeHtml(singleTitle)}</div>
        ` : ''}

        ${riskBannerHtml}

        ${targetBoxHtml}

        ${diffHtml}

        ${contentHtml}
      </div>

      <div class="interaction-card-actions" style="display: flex; align-items: center; gap: 10px; margin-top: 8px; padding-top: 8px; border-top: 1px solid var(--border-color, rgba(255,255,255,0.08)); flex-shrink: 0;">
        <button type="button" id="btn-interaction-skip" class="btn-interaction-skip" ${isSubmittingInteraction ? 'disabled' : ''} style="flex: 1; text-align: center; padding: 8px 14px; border-radius: 8px; border: 1px solid var(--border-color); background: transparent; color: var(--text-secondary); cursor: pointer;">跳过</button>
        <button type="button" id="btn-interaction-submit" class="btn-interaction-submit" ${(isSubmittingInteraction || !canSubmit) ? 'disabled' : ''} style="flex: 1; text-align: center; padding: 8px 16px; border-radius: 8px; border: none; background: #2563eb; color: white; font-weight: 600; cursor: ${(isSubmittingInteraction || !canSubmit) ? 'not-allowed' : 'pointer'}; opacity: ${(isSubmittingInteraction || !canSubmit) ? '0.45' : '1'};">
          <span>${escapeHtml(submitLabelText)}</span>
        </button>
      </div>
    </div>
  `;

  container.classList.remove("hidden");

  const bodyEl = container.querySelector(".interaction-card-body");
  const expandBtn = container.querySelector("#btn-interaction-expand");
  if (bodyEl) {
    if (prevScrollTop > 0) {
      bodyEl.scrollTop = prevScrollTop;
    }
    if (expandBtn && (isInteractionCardExpanded || bodyEl.scrollHeight > bodyEl.clientHeight + 4)) {
      expandBtn.style.display = "inline-flex";
    }
  }
  if (expandBtn) {
    expandBtn.addEventListener("click", () => {
      isInteractionCardExpanded = !isInteractionCardExpanded;
      renderInteractionCard(false);
    });
  }
  const diffToggleBtn = container.querySelector("#btn-interaction-diff-toggle");
  if (diffToggleBtn) {
    diffToggleBtn.addEventListener("click", () => {
      isInteractionDiffExpanded = !isInteractionDiffExpanded;
      renderInteractionCard(false);
    });
  }

  // Multi-question option click handlers
  container.querySelectorAll(".interaction-option-item[data-q-idx]").forEach(item => {
    item.addEventListener("click", (e) => {
      if (e.target && e.target.classList.contains("interaction-multi-write-in-input")) {
        return;
      }
      const qIdx = parseInt(item.dataset.qIdx, 10);
      const optId = item.dataset.optId;
      const isQMulti = item.dataset.isMulti === "1";
      const isWriteIn = item.dataset.isWritein === "1";
      if (!optId) return;

      if (isQMulti) {
        if (isWriteIn) {
          if (multiQuestionWriteInActives[qIdx]) {
            if (e.target && e.target.closest(".js-toggle-writein-off")) {
              multiQuestionWriteInActives[qIdx] = false;
              renderInteractionCard();
            } else {
              const input = container.querySelector(`.interaction-multi-write-in-input[data-q-idx="${qIdx}"]`);
              if (input) input.focus();
            }
            return;
          }
          multiQuestionWriteInActives[qIdx] = true;
          renderInteractionCard();
          setTimeout(() => {
            const input = container.querySelector(`.interaction-multi-write-in-input[data-q-idx="${qIdx}"]`);
            if (input) {
              input.focus();
              input.selectionStart = input.selectionEnd = input.value.length;
            }
          }, 30);
        } else {
          const cur = multiQuestionMultiSelections[qIdx] || [];
          multiQuestionMultiSelections[qIdx] = cur.includes(optId)
            ? cur.filter(id => id !== optId)
            : [...cur, optId];
          renderInteractionCard();
        }
        return;
      }

      multiQuestionSelections[qIdx] = optId;
      renderInteractionCard();
      if (isWriteIn) {
        setTimeout(() => {
          const input = container.querySelector(`.interaction-multi-write-in-input[data-q-idx="${qIdx}"]`);
          if (input) {
            input.focus();
            input.selectionStart = input.selectionEnd = input.value.length;
          }
        }, 30);
      }
    });
  });

  // Multi-question write-in inputs
  container.querySelectorAll(".interaction-multi-write-in-input").forEach(input => {
    input.addEventListener("input", (e) => {
      const qIdx = parseInt(input.dataset.qIdx, 10);
      multiQuestionWriteIns[qIdx] = e.target.value;
      const submitBtnEl = container.querySelector("#btn-interaction-submit");
      if (submitBtnEl && currentPendingInteraction?.questions) {
        const ready = currentPendingInteraction.questions.every((q, idx) => {
          if (!q.isMultiSelect) return true;
          const sel = multiQuestionMultiSelections[idx] || [];
          const hasW = Boolean(multiQuestionWriteInActives[idx]) && Boolean((multiQuestionWriteIns[idx] || "").trim());
          return sel.length > 0 || hasW;
        });
        submitBtnEl.disabled = isSubmittingInteraction || !ready;
        submitBtnEl.style.opacity = submitBtnEl.disabled ? "0.45" : "1";
        submitBtnEl.style.cursor = submitBtnEl.disabled ? "not-allowed" : "pointer";
      }
    });
  });

  // Single-question option click handlers
  container.querySelectorAll(".interaction-option-item:not([data-q-idx])").forEach(item => {
    item.addEventListener("click", (e) => {
      if (e.target && e.target.id === "interaction-write-in-input") {
        return;
      }
      const optId = item.dataset.optId;
      const isMulti = item.dataset.isMulti === "1";
      const isWriteIn = item.dataset.isWritein === "1";
      if (!optId) return;

      if (isMulti) {
        if (isWriteIn) {
          if (singleWriteInActive) {
            if (e.target && e.target.closest(".js-toggle-writein-off")) {
              singleWriteInActive = false;
              renderInteractionCard();
            } else {
              const input = container.querySelector("#interaction-write-in-input");
              if (input) input.focus();
            }
            return;
          }
          singleWriteInActive = true;
          renderInteractionCard();
          setTimeout(() => {
            const input = container.querySelector("#interaction-write-in-input");
            if (input) {
              input.focus();
              input.selectionStart = input.selectionEnd = input.value.length;
            }
          }, 30);
        } else {
          singleMultiSelections = singleMultiSelections.includes(optId)
            ? singleMultiSelections.filter(id => id !== optId)
            : [...singleMultiSelections, optId];
          renderInteractionCard();
        }
        return;
      }

      selectedInteractionOptionId = optId;
      renderInteractionCard();
      if (isWriteIn) {
        setTimeout(() => {
          const input = container.querySelector("#interaction-write-in-input");
          if (input) {
            input.focus();
            input.selectionStart = input.selectionEnd = input.value.length;
          }
        }, 30);
      }
    });
  });

  const writeInInput = container.querySelector("#interaction-write-in-input");
  if (writeInInput) {
    writeInInput.addEventListener("input", (e) => {
      singleWriteInText = e.target.value;
      if (isSingleMultiSelect) {
        const submitBtnEl = container.querySelector("#btn-interaction-submit");
        if (submitBtnEl) {
          const cnt = singleMultiSelections.length + (singleWriteInActive && singleWriteInText.trim().length > 0 ? 1 : 0);
          submitBtnEl.disabled = isSubmittingInteraction || cnt === 0;
          submitBtnEl.style.opacity = submitBtnEl.disabled ? "0.45" : "1";
          submitBtnEl.style.cursor = submitBtnEl.disabled ? "not-allowed" : "pointer";
          const span = submitBtnEl.querySelector("span");
          if (span && !isSubmittingInteraction) {
            span.textContent = cnt > 0 ? `${tr("提交")} (${cnt})` : tr("提交");
          }
        }
      }
    });
    writeInInput.addEventListener("keydown", (e) => {
      if (e.key === "Enter") {
        e.preventDefault();
        handleInteractionSubmit(false);
      }
    });
  }

  const skipBtn = container.querySelector("#btn-interaction-skip");
  if (skipBtn) {
    skipBtn.addEventListener("click", () => handleInteractionSubmit(true));
  }

  const submitBtn = container.querySelector("#btn-interaction-submit");
  if (submitBtn) {
    submitBtn.addEventListener("click", () => handleInteractionSubmit(false));
  }
}

async function handleInteractionSubmit(isSkip) {
  if (isSubmittingInteraction || !currentPendingInteraction || !activeCascadeId) return;

  const interaction = currentPendingInteraction;
  const hasMultiQuestions = interaction.questions && interaction.questions.length > 1;
  const isSingleMultiSelect = !hasMultiQuestions && Boolean(interaction.isMultiSelect || interaction.questions?.[0]?.isMultiSelect);

  if (!isSkip) {
    if (hasMultiQuestions) {
      const ready = interaction.questions.every((q, idx) => {
        if (!q.isMultiSelect) return true;
        const sel = multiQuestionMultiSelections[idx] || [];
        const hasW = Boolean(multiQuestionWriteInActives[idx]) && Boolean((multiQuestionWriteIns[idx] || "").trim());
        return sel.length > 0 || hasW;
      });
      if (!ready) return;
    } else if (isSingleMultiSelect) {
      const cnt = singleMultiSelections.length + (singleWriteInActive && singleWriteInText.trim().length > 0 ? 1 : 0);
      if (cnt === 0) return;
    }
  }

  isSubmittingInteraction = true;
  renderInteractionCard();

  try {
    let questionResponses = null;
    let selectedOptionId = isSkip ? "" : (selectedInteractionOptionId || "");
    let writeInText = "";

    if (hasMultiQuestions) {
      questionResponses = interaction.questions.map((q, idx) => {
        if (isSkip) {
          return { questionIndex: idx, selectedOptionIds: [], writeInResponse: "", skipped: true };
        }
        if (q.isMultiSelect) {
          const sel = multiQuestionMultiSelections[idx] || [];
          const orderedIds = (q.options || []).map(o => o.id).filter(id => sel.includes(id));
          const writeIn = multiQuestionWriteInActives[idx] ? (multiQuestionWriteIns[idx] || "").trim() : "";
          return {
            questionIndex: idx,
            selectedOptionIds: orderedIds,
            writeInResponse: writeIn,
            skipped: false
          };
        }
        const optId = multiQuestionSelections[idx] || q.defaultOptionId || q.options?.[0]?.id || "1";
        const isW = optId === "__write_in__" || Boolean((q.options || []).find(o => o.id === optId)?.isDeny);
        const writeIn = isW ? (multiQuestionWriteIns[idx] || "").trim() : "";
        return {
          questionIndex: idx,
          selectedOptionIds: isW ? [] : [optId],
          writeInResponse: writeIn,
          skipped: false
        };
      });
      selectedOptionId = questionResponses[0]?.selectedOptionIds?.[0] || "";
    } else if (isSingleMultiSelect) {
      if (isSkip) {
        questionResponses = [{ questionIndex: 0, selectedOptionIds: [], writeInResponse: "", skipped: true }];
        selectedOptionId = "";
        writeInText = "";
      } else {
        const orderedIds = (interaction.options || []).map(o => o.id).filter(id => singleMultiSelections.includes(id));
        const trimmedWriteIn = singleWriteInActive ? singleWriteInText.trim() : "";
        questionResponses = [{
          questionIndex: 0,
          selectedOptionIds: orderedIds,
          writeInResponse: trimmedWriteIn,
          skipped: false
        }];
        selectedOptionId = orderedIds[0] || (trimmedWriteIn ? "__write_in__" : "");
        writeInText = trimmedWriteIn;
      }
    } else {
      const writeInInput = document.getElementById("interaction-write-in-input");
      writeInText = writeInInput ? writeInInput.value.trim() : "";
    }

    const selectedOpt = interaction.options?.find(o => o.id === selectedOptionId);
    const isPermissionInteraction = interaction.type === "permission" || interaction.type === "file_permission";
    const isDeny = Boolean(selectedOpt?.isDeny) || (isPermissionInteraction && (selectedOptionId === "5" || selectedOptionId === "__write_in__"));

    const payload = {
      cascadeId: activeCascadeId,
      trajectoryId: interaction.trajectoryId || "",
      stepIndex: interaction.stepIndex,
      substepIndex: interaction.substepIndex || 0,
      type: interaction.type,
      optionId: selectedOptionId,
      scope: selectedOpt?.scope || 1,
      allow: isSkip ? false : !isDeny,
      writeInResponse: isSkip ? "" : writeInText,
      target: interaction.target || "",
      skipped: isSkip,
      questionResponses: questionResponses
    };

    const res = await fetch("/gateway/cascade/interaction", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload)
    });

    const data = await res.json();
    if (!res.ok || data.status === "error") {
      throw new Error(data.error || "提交交互选择失败");
    }

    // Success: clear pending interaction
    singleMultiSelections = [];
    singleWriteInActive = false;
    multiQuestionSelections = {};
    multiQuestionMultiSelections = {};
    multiQuestionWriteInActives = {};
    multiQuestionWriteIns = {};
    singleWriteInText = "";
    updatePendingInteraction(null, false);
    if (activeCascadeId && currentTrajectories[activeCascadeId]) {
      currentTrajectories[activeCascadeId].needsInput = false;
    }

    // Prompt stream/chat refresh
    if (activeCascadeId) {
      loadChat(activeCascadeId, true);
    }
  } catch (err) {
    alert(typeof t === "function" ? t("提交失败: " + err.message) : "提交失败: " + err.message);
  } finally {
    isSubmittingInteraction = false;
    if (currentPendingInteraction) {
      renderInteractionCard();
    }
  }
}

async function handleInteractionSubmitDirect(optionId, writeInText) {
  if (isSubmittingInteraction || !currentPendingInteraction || !activeCascadeId) return;
  isSubmittingInteraction = true;
  const interaction = currentPendingInteraction;
  const selectedOpt = interaction.options?.find(o => o.id === optionId);
  const isDeny = optionId === "deny" || optionId === "deny_with_feedback";
  const payload = {
    cascadeId: activeCascadeId,
    trajectoryId: interaction.trajectoryId || "",
    stepIndex: interaction.stepIndex,
    substepIndex: interaction.substepIndex || 0,
    type: interaction.type,
    optionId: optionId,
    scope: selectedOpt?.scope || 1,
    allow: !isDeny,
    writeInResponse: writeInText || "",
    target: interaction.target || "",
    skipped: false,
    questionResponses: null
  };
  try {
    const res = await fetch("/gateway/cascade/interaction", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload)
    });
    if (!res.ok) {
      const data = await res.json().catch(() => ({}));
      throw new Error(data.error || "提交审批选择失败");
    }
    updatePendingInteraction(null, false);
    if (activeCascadeId && currentTrajectories[activeCascadeId]) {
      currentTrajectories[activeCascadeId].needsInput = false;
    }
    if (activeCascadeId) {
      loadChat(activeCascadeId, true);
    }
  } catch (err) {
    alert(typeof t === "function" ? t("审批提交失败: " + err.message) : "审批提交失败: " + err.message);
  } finally {
    isSubmittingInteraction = false;
    if (currentPendingInteraction) {
      renderInteractionCard();
    }
  }
}

let pjsDimTimer = null;
let pjsHideTimer = null;
let pjsLastScrollTop = 0;

function wakePromptJumpStepper() {
  const stepperEl = document.getElementById("prompt-jump-stepper");
  if (!stepperEl) return;
  stepperEl.classList.remove("hidden");
  stepperEl.classList.remove("dimmed");

  if (pjsDimTimer) clearTimeout(pjsDimTimer);
  if (pjsHideTimer) clearTimeout(pjsHideTimer);

  // 停止滑动 3s 后，渐隐至 30%
  pjsDimTimer = setTimeout(() => {
    stepperEl.classList.add("dimmed");
  }, 3000);

  // 停止滑动 10s 后，完全隐藏
  pjsHideTimer = setTimeout(() => {
    stepperEl.classList.add("hidden");
  }, 10000);
}

function dimPromptJumpStepperOnScrollDown() {
  const stepperEl = document.getElementById("prompt-jump-stepper");
  if (!stepperEl) return;
  stepperEl.classList.add("dimmed");

  if (pjsDimTimer) clearTimeout(pjsDimTimer);
  if (pjsHideTimer) clearTimeout(pjsHideTimer);

  pjsHideTimer = setTimeout(() => {
    stepperEl.classList.add("hidden");
  }, 10000);
}

function hidePromptJumpStepperImmediately() {
  const stepperEl = document.getElementById("prompt-jump-stepper");
  if (!stepperEl) return;
  if (pjsDimTimer) clearTimeout(pjsDimTimer);
  if (pjsHideTimer) clearTimeout(pjsHideTimer);
  stepperEl.classList.add("hidden");
}

function initPromptJumpStepper() {
  const stepperEl = document.getElementById("prompt-jump-stepper");
  if (!stepperEl || stepperEl.dataset.inited) return;
  stepperEl.dataset.inited = "true";

  const btnUp = document.getElementById("pjs-btn-up");
  const btnDown = document.getElementById("pjs-btn-down");
  const streamEl = document.getElementById("messages-stream");

  if (btnUp) {
    btnUp.addEventListener("click", () => {
      wakePromptJumpStepper();
      const userRows = Array.from(streamEl.querySelectorAll(".message-row.user"));
      if (userRows.length < 2) return;
      const currentIdx = getVisibleTurnIndex(userRows, streamEl);
      const targetIdx = Math.max(0, currentIdx - 1);
      userRows[targetIdx]?.scrollIntoView({ behavior: "smooth", block: "start" });
      setTimeout(() => updatePromptJumpStepperStateOnly(), 350);
    });
  }

  if (btnDown) {
    let longPressTimer = null;
    let isLongPress = false;

    const handleDownAction = () => {
      wakePromptJumpStepper();
      const userRows = Array.from(streamEl.querySelectorAll(".message-row.user"));
      if (userRows.length < 2) return;
      const currentIdx = getVisibleTurnIndex(userRows, streamEl);
      if (currentIdx < userRows.length - 1) {
        const targetIdx = currentIdx + 1;
        userRows[targetIdx]?.scrollIntoView({ behavior: "smooth", block: "start" });
      } else {
        streamEl.scrollTo({ top: streamEl.scrollHeight, behavior: "smooth" });
      }
      setTimeout(() => updatePromptJumpStepperStateOnly(), 350);
    };

    btnDown.addEventListener("click", () => {
      if (isLongPress) return;
      handleDownAction();
    });

    const startLongPress = () => {
      isLongPress = false;
      longPressTimer = setTimeout(() => {
        isLongPress = true;
        wakePromptJumpStepper();
        streamEl.scrollTo({ top: streamEl.scrollHeight, behavior: "smooth" });
        setTimeout(() => updatePromptJumpStepperStateOnly(), 350);
      }, 450);
    };

    const cancelLongPress = () => {
      if (longPressTimer) {
        clearTimeout(longPressTimer);
        longPressTimer = null;
      }
    };

    btnDown.addEventListener("touchstart", startLongPress, { passive: true });
    btnDown.addEventListener("touchend", cancelLongPress, { passive: true });
    btnDown.addEventListener("touchcancel", cancelLongPress, { passive: true });
    btnDown.addEventListener("mousedown", startLongPress);
    btnDown.addEventListener("mouseup", cancelLongPress);
    btnDown.addEventListener("mouseleave", cancelLongPress);
  }
}

function getVisibleTurnIndex(userRows, streamEl, preReadScrollTop, preReadStreamOffsetTop) {
  if (!userRows || userRows.length === 0) return 0;
  const scrollTop = preReadScrollTop !== undefined ? preReadScrollTop : streamEl.scrollTop;
  const streamOffsetTop = preReadStreamOffsetTop !== undefined ? preReadStreamOffsetTop : streamEl.offsetTop;
  let bestIdx = 0;
  for (let i = 0; i < userRows.length; i++) {
    const rowTop = userRows[i].offsetTop - streamOffsetTop;
    if (rowTop <= scrollTop + 60) {
      bestIdx = i;
    } else {
      break;
    }
  }
  return bestIdx;
}

function applyPromptJumpStepperButtons(currentIdx, totalTurns) {
  const isAtTop = currentIdx <= 0;
  const isAtLastTurn = currentIdx >= totalTurns - 1;

  const btnUp = document.getElementById("pjs-btn-up");
  const iconDown = document.getElementById("pjs-icon-down");
  const iconBottom = document.getElementById("pjs-icon-bottom");

  if (btnUp) {
    btnUp.disabled = isAtTop;
    btnUp.classList.toggle("disabled", isAtTop);
  }

  if (iconDown && iconBottom) {
    if (isAtLastTurn) {
      iconDown.classList.add("hidden");
      iconBottom.classList.remove("hidden");
    } else {
      iconDown.classList.remove("hidden");
      iconBottom.classList.add("hidden");
    }
  }
}

function updatePromptJumpStepperStateOnly() {
  const stepperEl = document.getElementById("prompt-jump-stepper");
  const streamEl = document.getElementById("messages-stream");
  if (!stepperEl || !streamEl) return;

  const userRows = streamEl.querySelectorAll(".message-row.user");
  const totalTurns = userRows.length;
  if (totalTurns < 2) return;

  const currentIdx = getVisibleTurnIndex(userRows, streamEl);
  applyPromptJumpStepperButtons(currentIdx, totalTurns);
}

function updatePromptJumpStepper() {
  const stepperEl = document.getElementById("prompt-jump-stepper");
  const streamEl = document.getElementById("messages-stream");
  const chatInput = document.getElementById("chat-input");
  if (!stepperEl || !streamEl) return;

  initPromptJumpStepper();

  // Read all layout metrics before mutating DOM classes
  const userRows = streamEl.querySelectorAll(".message-row.user");
  const totalTurns = userRows.length;
  const isInputFocused = chatInput && document.activeElement === chatInput;
  const scrollTop = streamEl.scrollTop;
  const dist = streamEl.scrollHeight - scrollTop - streamEl.clientHeight;
  const isNearBottom = dist <= 120;

  if (totalTurns < 2 || isNearBottom || isInputFocused) {
    hidePromptJumpStepperImmediately();
    return;
  }

  const currentIdx = getVisibleTurnIndex(userRows, streamEl, scrollTop, streamEl.offsetTop);
  applyPromptJumpStepperButtons(currentIdx, totalTurns);
}

function initScrollListener() {
  const streamEl = document.getElementById("messages-stream");
  if (!streamEl || streamEl.dataset.hasScrollListener) return;
  streamEl.dataset.hasScrollListener = "true";

  initPromptJumpStepper();

  const updateNearBottom = () => {
    const dist = streamEl.scrollHeight - streamEl.scrollTop - streamEl.clientHeight;
    userIsNearBottom = dist <= 120;
    if (userIsNearBottom) {
      hidePromptJumpStepperImmediately();
    }
  };

  let scrollRafPending = false;
  streamEl.addEventListener("scroll", () => {
    if (scrollRafPending) return;
    scrollRafPending = true;
    requestAnimationFrame(() => {
      scrollRafPending = false;
      // Phase 1: Batch all layout/geometry reads together (zero interleaved DOM writes)
      const currentScrollTop = streamEl.scrollTop;
      const dist = streamEl.scrollHeight - currentScrollTop - streamEl.clientHeight;
      const streamOffsetTop = streamEl.offsetTop;
      const userRows = streamEl.querySelectorAll(".message-row.user");
      const totalTurns = userRows.length;
      const nearBottom = dist <= 120;
      const currentIdx = (!nearBottom && totalTurns >= 2)
        ? getVisibleTurnIndex(userRows, streamEl, currentScrollTop, streamOffsetTop)
        : 0;

      const deltaY = currentScrollTop - pjsLastScrollTop;
      pjsLastScrollTop = currentScrollTop;
      userIsNearBottom = nearBottom;

      // Phase 2: Perform all DOM class/attribute writes after reads complete
      if (totalTurns < 2 || nearBottom) {
        hidePromptJumpStepperImmediately();
        return;
      }
      if (deltaY < -4) {
        // 往上滑（查看旧历史）-> 唤醒 100%，重置 3s 渐隐与 10s 全隐
        wakePromptJumpStepper();
      } else if (deltaY > 4) {
        // 往下滑（看新消息）-> 立即渐隐至 30%，重置 10s 全隐
        dimPromptJumpStepperOnScrollDown();
      }
      applyPromptJumpStepperButtons(currentIdx, totalTurns);
    });
  }, { passive: true });

  streamEl.addEventListener("touchstart", () => { isUserTouching = true; }, { passive: true });
  streamEl.addEventListener("touchend", () => {
    isUserTouching = false;
    updateNearBottom();
  }, { passive: true });
  streamEl.addEventListener("touchcancel", () => {
    isUserTouching = false;
    updateNearBottom();
  }, { passive: true });

  const chatInput = document.getElementById("chat-input");
  if (chatInput && !chatInput.dataset.hasStepperFocusListener) {
    chatInput.dataset.hasStepperFocusListener = "true";
    chatInput.addEventListener("focus", () => hidePromptJumpStepperImmediately());
    chatInput.addEventListener("blur", () => updatePromptJumpStepper());
  }
}

function closeActiveWs(resetBackoff = false) {
  if (resetBackoff) wsReconnectAttempts = 0;
  if (wsReconnectTimer) {
    clearTimeout(wsReconnectTimer);
    wsReconnectTimer = null;
  }
  if (activeWs) {
    activeWs.onopen = null;
    activeWs.onmessage = null;
    activeWs.onerror = null;
    activeWs.onclose = null;
    activeWs.close();
    activeWs = null;
  }
}

function updateChatControls(isRunning, wsUri, hasAction = false) {
  const sendBtn = document.getElementById("btn-send");
  const chatInput = document.getElementById("chat-input");
  const wsText = document.getElementById("chat-workspace-text");

  if (wsText) {
    if (wsUri) {
      // Prefer the gateway-resolved official project name/alias when wsUri belongs to the active conversation.
      let resolvedName = "";
      const activeSummary = typeof currentTrajectories !== "undefined" && activeCascadeId ? currentTrajectories[activeCascadeId] : null;
      if (activeSummary?.workspaceName) {
        const norm = (u) => String(u || "").replace(/\/+$/, "");
        const summaryUris = [
          ...(activeSummary.trajectoryMetadata?.workspaceUris || activeSummary.workspaceUris || []),
          ...(activeSummary.workspaces || []).map((w) => w?.workspaceFolderAbsoluteUri),
        ].filter(Boolean).map(norm);
        if (summaryUris.includes(norm(wsUri))) resolvedName = activeSummary.workspaceName;
      }
      const wsName = resolvedName || wsUri.split("/").filter(Boolean).pop() || "Chat";
      if (activeSummary?.isScheduledTask && wsName !== "Chat" && typeof window.renderScheduledAlarmIconHtml === "function") {
        wsText.innerHTML = `${escapeHtml(wsName)}${window.renderScheduledAlarmIconHtml()}`;
      } else {
        wsText.textContent = wsName;
      }
      wsText.title = wsUri;
    } else {
      wsText.textContent = "Chat";
      wsText.title = "新对话 · 不关联任何工作区";
    }
  }

  if (sendBtn) {
    const iconSend = sendBtn.querySelector(".icon-send");
    const iconStop = sendBtn.querySelector(".icon-stop");
    const iconVoice = sendBtn.querySelector(".icon-voice");
    const iconHangup = sendBtn.querySelector(".icon-hangup");
    const hasContent = (chatInput && chatInput.value.trim().length > 0) || (pendingImages && pendingImages.length > 0) || (typeof pendingAttachments !== "undefined" && pendingAttachments.length > 0) || !!getSelectedSlashCommand();
    const inVoiceCall = typeof WebVoiceCallManager !== "undefined" && WebVoiceCallManager.isInCall;
    const voiceEnabled = typeof WebVoiceCallManager !== "undefined" && WebVoiceCallManager.isVoiceModeEnabled();

    const showOnlyIcon = (activeIcon) => {
      [iconSend, iconStop, iconVoice, iconHangup].forEach(ic => {
        if (!ic) return;
        if (ic === activeIcon) ic.classList.remove("hidden");
        else ic.classList.add("hidden");
      });
    };

    const stopTaskBtnInVoice = document.getElementById("btn-voice-stop-task");
    if (stopTaskBtnInVoice) {
      stopTaskBtnInVoice.classList.toggle("hidden", !(inVoiceCall && isRunning));
    }

    if (inVoiceCall && !hasContent) {
      sendBtn.className = "btn-action-circle hangup-mode";
      sendBtn.title = typeof t === "function" ? t("挂断实时语音通话") : "挂断实时语音通话";
      sendBtn.setAttribute("aria-label", sendBtn.title);
      showOnlyIcon(iconHangup);
    } else if (hasContent || (!isRunning && !voiceEnabled)) {
      sendBtn.className = "btn-action-circle send-mode" + (hasContent ? " active" : "");
      sendBtn.title = isRunning
        ? (typeof t === "function" ? t("加入待发送队列") : "加入待发送队列")
        : (typeof t === "function" ? t("发送") : "发送");
      sendBtn.setAttribute("aria-label", sendBtn.title);
      showOnlyIcon(iconSend);
    } else if (isRunning) {
      sendBtn.className = "btn-action-circle stop-mode";
      sendBtn.title = typeof t === "function" ? t("停止任务") : "停止任务";
      sendBtn.setAttribute("aria-label", sendBtn.title);
      showOnlyIcon(iconStop);
    } else {
      sendBtn.className = "btn-action-circle voice-mode";
      sendBtn.title = typeof t === "function" ? t("实时语音对话") : "实时语音对话";
      sendBtn.setAttribute("aria-label", sendBtn.title);
      showOnlyIcon(iconVoice);
    }
  }
}

// Rebuilds a full stream payload from a delta frame (server sends delta=1 frames as changes
// against what this connection already received). The baseline lives on the socket, so every
// new connection starts clean from a complete "init" frame. Returns false when the baseline
// does not line up, in which case the caller reconnects to resync.
function applyStreamDelta(ws, data) {
  if (data.delta) {
    const prevSteps = ws.__steps;
    const prevMsgs = ws.__messages;
    if (!prevSteps && !prevMsgs) return false;

    if (Array.isArray(data.messageIds)) {
      const byId = new Map();
      (prevMsgs || []).forEach((m) => byId.set(m.id, m));
      (data.messages || []).forEach((m) => byId.set(m.id, m));
      const rebuilt = [];
      for (const id of data.messageIds) {
        const m = byId.get(id);
        if (!m) return false;
        rebuilt.push(m);
      }
      data.messages = rebuilt;
    }

    if (typeof data.stepsFrom === "number") {
      if (!prevSteps || data.stepsFrom > prevSteps.length) return false;
      const rebuilt = prevSteps.slice(0, data.stepsFrom).concat(data.steps || []);
      data.steps = typeof data.totalSteps === "number" ? rebuilt.slice(0, data.totalSteps) : rebuilt;
    }
  }
  ws.__steps = data.steps || ws.__steps || null;
  ws.__messages = data.messages || ws.__messages || null;
  return true;
}

let streamWsSeq = 0;
const streamResumeCache = new Map();
const MAX_STREAM_RESUME_CACHE = 16;
let activeLiveTextTranslations = {};
let activeLiveTextTranslationsCascadeId = "";

function setLiveTextTranslations(cascadeId, mapObj) {
  activeLiveTextTranslationsCascadeId = cascadeId || "";
  activeLiveTextTranslations = (mapObj && typeof mapObj === "object") ? mapObj : {};
}

function resolveLiveTranslatedText(raw) {
  if (typeof raw !== "string") return raw;
  const trimmed = raw.trim();
  if (!trimmed) return raw;
  if (activeLiveTextTranslationsCascadeId && activeCascadeId && activeLiveTextTranslationsCascadeId !== activeCascadeId) {
    return raw;
  }
  const translated = activeLiveTextTranslations[trimmed];
  return (typeof translated === "string" && translated.trim()) ? translated.trim() : raw;
}

function saveStreamResumeState(cascadeId, rev, steps, messages) {
  if (!cascadeId || typeof rev !== "number" || rev <= 0) return;
  if (streamResumeCache.has(cascadeId)) streamResumeCache.delete(cascadeId);
  streamResumeCache.set(cascadeId, { rev, steps, messages });
  if (streamResumeCache.size > MAX_STREAM_RESUME_CACHE) {
    const oldestKey = streamResumeCache.keys().next().value;
    if (oldestKey) streamResumeCache.delete(oldestKey);
  }
}

async function connectStreamWs(cascadeId) {
  if (activeWs && activeWs.__cascadeId === cascadeId && (activeWs.readyState === WebSocket.OPEN || activeWs.readyState === WebSocket.CONNECTING)) {
    return;
  }
  const mySeq = ++streamWsSeq;
  closeActiveWs();

  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  const webLangTag = window.AIAssist?.getTargetLangTag?.() || navigator.language || "";
  let wsUrl = `${proto}//${location.host}/gateway/cascade/stream?cascadeId=${encodeURIComponent(cascadeId)}&delta=1&messages=0`;
  if (webLangTag) {
    wsUrl += `&lang=${encodeURIComponent(webLangTag)}`;
  }
  const resumeState = streamResumeCache.get(cascadeId);
  if (resumeState && resumeState.rev > 0 && (resumeState.steps || resumeState.messages)) {
    wsUrl += `&sinceRev=${encodeURIComponent(String(resumeState.rev))}`;
  }
  try {
    // S9 / C-1: Exchange HttpOnly session cookie for a short-lived one-time ticket
    // so no long-lived token ever appears in query strings or logs.
    const resp = await originalFetch("/api/v1/auth/ws-ticket", { method: "POST" });
    if (resp.ok) {
      const data = await resp.json();
      if (data && data.ticket) {
        wsUrl += `&ticket=${encodeURIComponent(data.ticket)}`;
      }
    }
  } catch (_) {}

  // Abort if active cascade changed or another connectStreamWs superseded us during ticket exchange
  if (mySeq !== streamWsSeq || activeCascadeId !== cascadeId) return;
  closeActiveWs();

  try {
    const ws = new WebSocket(wsUrl);
    ws.__cascadeId = cascadeId;
    if (resumeState && resumeState.rev > 0) {
      ws.__steps = resumeState.steps || null;
      ws.__messages = resumeState.messages || null;
    }
    activeWs = ws;

    ws.onopen = () => {
      // WS successfully established: stop HTTP polling fallback & reset backoff
      wsReconnectAttempts = 0;
      if (pollTimer) {
        clearInterval(pollTimer);
        pollTimer = null;
      }
    };

    ws.onmessage = (event) => {
      if (activeCascadeId !== cascadeId) return;
      try {
        const data = JSON.parse(event.data);
        if (data.cascadeId !== cascadeId) return;

        if (data.type === "noop") {
          if (typeof data.rev === "number" && data.rev > 0) {
            saveStreamResumeState(cascadeId, data.rev, ws.__steps, ws.__messages);
          }
          return;
        }

        if (!applyStreamDelta(ws, data)) {
          // Baseline mismatch: clear cached rev, drop this connection, and let onclose reconnect for a fresh "init".
          console.warn("[WS] Delta baseline mismatch, resyncing");
          streamResumeCache.delete(cascadeId);
          ws.close();
          return;
        }
        if (typeof data.rev === "number" && data.rev > 0) {
          saveStreamResumeState(cascadeId, data.rev, ws.__steps, ws.__messages);
        }
        setLiveTextTranslations(cascadeId, data.liveTextTranslations || null);

        const alignedModel = data.desktopActiveModel || data.activeModel;
        if (alignedModel) {
          syncActiveModel(alignedModel);
        }

        const isRunning = data.status === "CASCADE_RUN_STATUS_RUNNING" || hasActiveRunningTasks(data.runningTasks);
        const hasPending = Boolean(data.pendingInteraction || (data.canProceed && !isRunning));

        if (data.contextMeter && typeof updateContextMeterUI === "function") {
          updateContextMeterUI(data.contextMeter, isRunning, hasPending);
        }

        if (typeof data.canProceed === "boolean") {
          currentCanProceed = data.canProceed && !isRunning;
          currentProceedArtifactUri = data.proceedArtifactUri || null;
          updateProceedButton(currentCanProceed);
        } else if (isRunning) {
          currentCanProceed = false;
          updateProceedButton(false);
        }

        if (data.pendingInteraction) {
          updatePendingInteraction(data.pendingInteraction, isRunning);
        } else {
          updatePendingInteraction(null, isRunning);
        }

        const hasAction = !!(data.pendingInteraction || currentCanProceed);
        updateChatControls(isRunning, data.workspaceUri, hasAction);
        if (data.type === "init" && data.workspaceUri) {
          window.V233?.checkWorkspaceTrust?.(cascadeId, data.workspaceUri);
        }

        if (data.title && document.getElementById("header-title")) {
          document.getElementById("header-title").textContent = data.title;
        }

        if (currentTrajectories[cascadeId]) {
          currentTrajectories[cascadeId].status = data.status;
          currentTrajectories[cascadeId].runningTasks = Array.isArray(data.runningTasks) ? data.runningTasks : [];
          currentTrajectories[cascadeId].stepCount = data.totalSteps;
          currentTrajectories[cascadeId].needsInput = hasAction;
        }

        if (data.queuedMessages !== undefined) {
          LocalQueueManager.syncFromServer(data.queuedMessages, data.steps || data.messages);
        } else if (data.steps) {
          LocalQueueManager.syncFromServer(null, data.steps);
        }

        RunningTasksManager.syncFromServer(data.runningTasks);
        SubagentManager.sync(data);
        if (LocalQueueManager.queue.length > 0) LocalQueueManager.render();
        if (typeof SwarmManager !== "undefined") SwarmManager.sync(data);
        if (window.Phase3?.syncStreamData) window.Phase3.syncStreamData(data);
        window.V233?.syncBranchState?.(cascadeId, data);

        const stepsToRender = data.steps || ws.__steps;
        if (stepsToRender) {
          scheduleRenderMessages(stepsToRender, isRunning);
        }
      } catch (err) {
        console.warn("[WS] Error parsing stream message:", err);
      }
    };

    ws.onerror = () => {
      fallbackToHttpPolling(cascadeId);
    };

    ws.onclose = () => {
      if (activeCascadeId === cascadeId) {
        fallbackToHttpPolling(cascadeId);
        const delay = Math.min(1000 * Math.pow(1.5, wsReconnectAttempts), 15000);
        wsReconnectAttempts++;
        wsReconnectTimer = setTimeout(() => {
          if (activeCascadeId === cascadeId) {
            connectStreamWs(cascadeId);
          }
        }, delay);
      }
    };
  } catch (e) {
    fallbackToHttpPolling(cascadeId);
  }
}

function fallbackToHttpPolling(cascadeId) {
  if (activeCascadeId === cascadeId && !pollTimer) {
    // Opening a chat relies on the stream's init frame; if the stream failed before anything
    // was rendered for this session, load it over HTTP right away instead of after the first tick.
    const streamEl = document.getElementById("messages-stream");
    if (streamEl && (streamEl.__currentCascadeId !== cascadeId || streamEl.querySelector(".loading-state"))) {
      loadChat(cascadeId, true);
    }
    startFallbackPoll(cascadeId);
  }
}

// HTTP stand-in for the stream: each poll downloads the whole raw trajectory (megabytes for long
// sessions), so poll every tick only while the agent is running and at most every 10s when idle.
function startFallbackPoll(cascadeId) {
  let lastPollAt = Date.now();
  pollTimer = setInterval(() => {
    if (activeCascadeId !== cascadeId || document.visibilityState !== "visible") return;
    const summary = currentTrajectories[cascadeId];
    const running = summary?.status === "CASCADE_RUN_STATUS_RUNNING" || hasActiveRunningTasks(summary?.runningTasks);
    if (!running && Date.now() - lastPollAt < 10000) return;
    lastPollAt = Date.now();
    loadChat(cascadeId, true);
  }, 1500);
}

async function loadChat(cascadeId, isBackgroundPoll = false) {
  const streamEl = document.getElementById("messages-stream");
  initScrollListener();

  if (!isBackgroundPoll && (!streamEl.children.length || streamEl.querySelector(".loading-state"))) {
    streamEl.innerHTML = `
      <div class="loading-state">
        <div class="spinner"></div>
        <p>正在同步会话历史与步骤...</p>
      </div>
    `;
  }

  try {
    const data = await rpc("GetCascadeTrajectory", { cascadeId });
    const traj = data.trajectory || {};
    const steps = traj.steps || [];

    const summary = currentTrajectories[cascadeId];
    // The list summary can be stale while the stream is down; the trajectory carries the live status.
    if (summary && data.status) summary.status = data.status;
    const isRunning = summary?.status === "CASCADE_RUN_STATUS_RUNNING" || hasActiveRunningTasks(summary?.runningTasks);
    const wsUri = traj.workspaceUris?.[0] || "";

    const dynamicTitle = formatConversationTitle(traj.annotations, traj.summary, "");
    if (dynamicTitle && document.getElementById("header-title")) {
      document.getElementById("header-title").textContent = dynamicTitle;
    }

    updateChatControls(isRunning, wsUri);
    window.V233?.checkWorkspaceTrust?.(cascadeId, wsUri);
    LocalQueueManager.init(cascadeId);
    RunningTasksManager.init(cascadeId);
    renderMessages(steps, isRunning);

    // P1 / B-4: Avoid redundant HTTP messages request when WebSocket stream is active
    if (!activeWs || activeWs.readyState !== WebSocket.OPEN) {
      const webLangTag = window.AIAssist?.getTargetLangTag?.() || navigator.language || "";
      const langQ = webLangTag ? `&lang=${encodeURIComponent(webLangTag)}` : "";
      fetch(`/gateway/cascade/messages?cascadeId=${encodeURIComponent(cascadeId)}&limit=1${langQ}`)
        .then(res => res.json())
        .then(info => {
          if (activeCascadeId === cascadeId) {
            setLiveTextTranslations(cascadeId, info.liveTextTranslations || null);
            if (info.liveTextTranslations && Object.keys(info.liveTextTranslations).length > 0) {
              scheduleRenderMessages(steps, isRunning);
            }
            const alignedModel = info.desktopActiveModel || info.activeModel;
            if (alignedModel) {
              syncActiveModel(alignedModel);
            }
            if (info.queuedMessages !== undefined) {
              LocalQueueManager.syncFromServer(info.queuedMessages, info.messages || info.steps);
            } else if (info.messages) {
              LocalQueueManager.syncFromServer(null, info.messages);
            }
            RunningTasksManager.syncFromServer(info.runningTasks);
            SubagentManager.sync(info);
            if (window.Phase3?.syncStreamData) window.Phase3.syncStreamData(info);
            window.V233?.syncBranchState?.(cascadeId, info);
            if (currentTrajectories[cascadeId]) {
              if (info.status) currentTrajectories[cascadeId].status = info.status;
              currentTrajectories[cascadeId].runningTasks = Array.isArray(info.runningTasks) ? info.runningTasks : [];
            }
            const liveRunning = isCascadeRunning(cascadeId);
            const liveHasPending = Boolean(info.pendingInteraction || (info.canProceed && !liveRunning));
            if (info.contextMeter && typeof updateContextMeterUI === "function") {
              updateContextMeterUI(info.contextMeter, liveRunning, liveHasPending);
            }
            currentCanProceed = !!info.canProceed && !liveRunning;
            currentProceedArtifactUri = info.proceedArtifactUri || null;
            updateProceedButton(currentCanProceed);
            updatePendingInteraction(info.pendingInteraction || null, liveRunning);
            const hasAction = !!(info.pendingInteraction || currentCanProceed);
            updateChatControls(liveRunning, wsUri, hasAction);
            if (currentTrajectories[cascadeId]) {
              currentTrajectories[cascadeId].needsInput = hasAction;
            }
          }
        })
        .catch(() => {});
    }

    if (isRunning && (!activeWs || activeWs.readyState !== WebSocket.OPEN) && !pollTimer) {
      startFallbackPoll(cascadeId);
    } else if (!isRunning && pollTimer && activeWs && activeWs.readyState === WebSocket.OPEN) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
  } catch (err) {
    if (!isBackgroundPoll && (!streamEl.children.length || streamEl.querySelector(".loading-state"))) {
      streamEl.innerHTML = `
        <div class="loading-state">
          <p style="color: var(--status-error);">加载会话失败: ${escapeHtml(err.message)}</p>
        </div>
      `;
    }
  }
}

const STEP_PLAN_VALID_STATUSES = new Set(["pending", "in_progress", "done", "skipped", "failed"]);
const STEP_PLAN_MD_STATUS_MAP = { " ": "pending", ">": "in_progress", x: "done", X: "done", "-": "skipped", "!": "failed" };
const STEP_PLAN_MD_LINE_RE = /^\s*[-*]\s+\[([ >xX\-!])\]\s+(.+?)\s*$/;

function extractToolNameFromRawStep(s) {
  if (!s) return "";
  if (s.metadata?.toolCall?.name) return s.metadata.toolCall.name;
  if (s.toolCall?.name) return s.toolCall.name;
  return "";
}

function parseStepPlanMarkdownWeb(md) {
  if (!md || typeof md !== "string") return null;
  const items = [];
  for (const line of md.split("\n")) {
    const m = STEP_PLAN_MD_LINE_RE.exec(line);
    if (m) {
      const title = m[2].trim();
      if (title) {
        items.push({ title, status: STEP_PLAN_MD_STATUS_MAP[m[1]] || "pending" });
      }
    }
  }
  return items.length > 0 ? items : null;
}

function parseStepPlanJsonWeb(raw) {
  let val = raw;
  if (typeof val === "string") {
    try { val = JSON.parse(val); } catch { return null; }
  }
  if (val && typeof val === "object" && !Array.isArray(val) && "steps" in val) {
    return parseStepPlanJsonWeb(val.steps);
  }
  if (!Array.isArray(val)) return null;
  const items = [];
  for (const d of val) {
    if (!d || typeof d !== "object") continue;
    const title = typeof d.title === "string" ? d.title.trim() : "";
    if (!title) continue;
    const status = typeof d.status === "string" && STEP_PLAN_VALID_STATUSES.has(d.status) ? d.status : "pending";
    items.push({ title, status });
  }
  return items.length > 0 ? items : null;
}

function finalizeStepPlanWeb(anchorStepIndex, rawItems) {
  if (!rawItems || rawItems.length === 0) return null;
  const seen = new Set();
  let doneCount = 0;
  let skippedCount = 0;
  let failedCount = 0;
  const items = rawItems.map(it => {
    const base = it.title.trim().toLowerCase().replace(/\s+/g, " ");
    let key = base;
    for (let k = 2; seen.has(key); k++) key = `${base}#${k}`;
    seen.add(key);
    if (it.status === "done") doneCount++;
    else if (it.status === "skipped") skippedCount++;
    else if (it.status === "failed") failedCount++;
    return { key, title: resolveLiveTranslatedText(it.title), status: it.status };
  });
  return {
    anchorStepIndex,
    items,
    doneCount,
    totalCount: items.length - skippedCount,
    skippedCount,
    failedCount
  };
}

function extractStepPlanFromRawStep(s, idx) {
  if (extractToolNameFromRawStep(s) !== "update_steps") return null;
  if (s.status === "CORTEX_STEP_STATUS_ERROR" || s.error?.shortError || s.errorMessage?.error?.shortError) return null;
  const md = s.generic?.result?.stepRenderInfo?.markdown;
  let parsed = parseStepPlanMarkdownWeb(md);
  if (s.status === "CORTEX_STEP_STATUS_CLEARED") {
    return finalizeStepPlanWeb(idx, parsed);
  }
  if (!parsed && s.generic?.args && "steps" in s.generic.args) {
    parsed = parseStepPlanJsonWeb(s.generic.args.steps);
  }
  if (!parsed) {
    const argsJson = s.metadata?.toolCall?.argumentsJson || s.toolCall?.argumentsJson || "";
    if (argsJson) parsed = parseStepPlanJsonWeb(argsJson);
  }
  return finalizeStepPlanWeb(idx, parsed);
}

function extractFocusFromRawStep(s) {
  if (extractToolNameFromRawStep(s) !== "update_focus") return "";
  if (s.status === "CORTEX_STEP_STATUS_ERROR" || s.error?.shortError || s.errorMessage?.error?.shortError) return "";
  const title = s.generic?.result?.stepRenderInfo?.title;
  if (typeof title === "string" && title.trim()) return resolveLiveTranslatedText(title.trim());
  const argFocus = s.generic?.args?.focus;
  if (argFocus !== undefined && argFocus !== null) {
    let val = argFocus;
    if (typeof val === "string") {
      try { const unq = JSON.parse(val); if (typeof unq === "string") val = unq; } catch {}
      if (val.trim()) return resolveLiveTranslatedText(val.trim());
    }
  }
  const argsJson = s.metadata?.toolCall?.argumentsJson || s.toolCall?.argumentsJson || "";
  if (argsJson) {
    try {
      const parsed = JSON.parse(argsJson);
      if (typeof parsed?.focus === "string" && parsed.focus.trim()) return resolveLiveTranslatedText(parsed.focus.trim());
    } catch {}
  }
  return "";
}

function stepCreatedTimestampWeb(s) {
  return (s?.metadata?.createdAt || s?.metadata?.startedAt || s?.startedAt || "").trim();
}

function formatElapsedDurationWeb(ms) {
  const secs = Math.max(0, Math.floor(ms / 1000));
  const h = Math.floor(secs / 3600);
  const m = Math.floor((secs % 3600) / 60);
  const s = secs % 60;
  if (h > 0) return `${h}h ${m}m ${s}s`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}

let _focusElapsedIntervalId = null;
function syncFocusElapsedTimers() {
  const els = document.querySelectorAll(".focus-elapsed-timer[data-started-at]");
  if (!els.length) {
    if (_focusElapsedIntervalId) {
      clearInterval(_focusElapsedIntervalId);
      _focusElapsedIntervalId = null;
    }
    return;
  }
  const updateAll = () => {
    const nodes = document.querySelectorAll(".focus-elapsed-timer[data-started-at]");
    if (!nodes.length) {
      if (_focusElapsedIntervalId) {
        clearInterval(_focusElapsedIntervalId);
        _focusElapsedIntervalId = null;
      }
      return;
    }
    const now = Date.now();
    nodes.forEach(el => {
      const ts = Number(el.getAttribute("data-started-at"));
      if (Number.isFinite(ts) && ts > 0) {
        el.textContent = formatElapsedDurationWeb(now - ts);
      }
    });
  };
  updateAll();
  if (!_focusElapsedIntervalId) {
    _focusElapsedIntervalId = setInterval(updateAll, 1000);
  }
}

function getDesktopUiLangWeb() {
  if (window.I18n?.isZh?.()) return "zh";
  if (window.I18n?.isJa?.()) return "ja";
  return "en";
}

function shortThinkingDurationWeb(raw) {
  const trimmed = String(raw || "").trim();
  if (!trimmed) return "";
  if (trimmed.endsWith("s") && !trimmed.endsWith("ms") && !trimmed.includes("m")) {
    const secs = Number(trimmed.slice(0, -1).trim());
    if (Number.isFinite(secs)) {
      const total = Math.max(1, Math.floor(secs));
      if (total >= 60) {
        const m = Math.floor(total / 60);
        const s = total % 60;
        return s > 0 ? `${m}m ${s}s` : `${m}m`;
      }
      return `${total}s`;
    }
  }
  return trimmed;
}

function formatThinkingTitleWeb(duration, isRunning) {
  const lang = getDesktopUiLangWeb();
  if (isRunning) {
    return lang === "en" ? "Thinking..." : "思考中...";
  }
  const dur = shortThinkingDurationWeb(duration);
  if (dur) {
    if (lang === "zh") return `已思考 ${dur}`;
    if (lang === "ja") return `${dur} 思考`;
    return `Thought for ${dur}`;
  }
  if (lang === "zh") return "思考过程";
  if (lang === "ja") return "思考プロセス";
  return "Thought Process";
}

function localizeDesktopPrefixWeb(raw) {
  const p = String(raw || "").trim();
  const lang = getDesktopUiLangWeb();
  if (!p || lang === "en") return p;
  const zh = lang === "zh";
  switch (p.toLowerCase()) {
    case "analyzed": return zh ? "已分析" : "分析済み";
    case "analyzing": return zh ? "正在分析" : "分析中";
    case "read": return zh ? "已读取" : "読み取り済み";
    case "reading": return zh ? "正在读取" : "読み取り中";
    case "edited": return zh ? "已编辑" : "編集済み";
    case "editing": return zh ? "正在编辑" : "編集中";
    case "created": return zh ? "已创建" : "作成済み";
    case "creating": return zh ? "正在创建" : "作成中";
    case "deleted": return zh ? "已删除" : "削除済み";
    case "deleting": return zh ? "正在删除" : "削除中";
    case "ran": return zh ? "已运行" : "実行済み";
    case "running": return zh ? "正在运行" : "実行中";
    case "run": return zh ? "运行" : "実行";
    case "searched": return zh ? "已搜索" : "検索済み";
    case "searching": return zh ? "正在搜索" : "検索中";
    case "listed": return zh ? "已浏览" : "一覧取得済み";
    case "listing": return zh ? "正在浏览" : "一覧取得中";
    case "rejected": return zh ? "已拒绝" : "拒否済み";
    case "canceled": return zh ? "已取消" : "キャンセル済み";
    case "interrupted": return zh ? "已中断" : "中断";
    case "errored": return zh ? "执行出错" : "エラー";
    case "canceled edit to": return zh ? "已取消编辑" : "編集キャンセル";
    case "canceled creation of": return zh ? "已取消创建" : "作成キャンセル";
    case "canceled deletion of": return zh ? "已取消删除" : "削除キャンセル";
    default: return p;
  }
}

const GENERIC_CMD_NOISE_RE = /^(running|run|execute|executing)?\s*(a\s*)?(shell\s*|bash\s*|terminal\s*)?command(s)?$/i;

function isGenericCmdNoiseWeb(s) {
  const t = String(s || "").trim();
  return !t || GENERIC_CMD_NOISE_RE.test(t);
}

function basenameOfUriWeb(raw) {
  if (!raw) return "";
  let s = String(raw).trim().replace(/^"|"$/g, "");
  s = s.replace(/^file:\/\//i, "").split("#")[0].split("?")[0].replace(/[/\\]+$/, "");
  try { s = decodeURIComponent(s); } catch {}
  return s.split(/[/\\]/).pop() || s;
}

function normalizeFileUriWeb(raw) {
  if (!raw) return "";
  const s = String(raw).trim().replace(/^"|"$/g, "");
  if (!s) return "";
  if (/^file:\/\//i.test(s)) return s;
  if (s.startsWith("/")) return `file://${s}`;
  return s;
}

function lineRangeSuffixWeb(startLine, endLine) {
  const s = Number(startLine) || 0;
  const e = Number(endLine) || 0;
  if (s > 0 && e > 0 && e !== s) return `#L${s}-${e}`;
  if (s > 0) return `#L${s}`;
  if (e > 0) return `#L1-${e}`;
  return "";
}

function truncateTailOutputWeb(raw, existingAbove = 0, maxLines = 80, maxChars = 4096) {
  if (!raw) return { output: "", numLinesAbove: existingAbove };
  let lines = String(raw).replace(/\r\n/g, "\n").split("\n");
  let extra = 0;
  if (lines.length > maxLines) {
    extra = lines.length - maxLines;
    lines = lines.slice(extra);
  }
  let out = lines.join("\n");
  if (out.length > maxChars) {
    const cut = out.length - maxChars;
    const nl = out.indexOf("\n", cut);
    if (nl >= 0 && nl + 1 < out.length) {
      extra += (out.slice(0, nl + 1).match(/\n/g) || []).length;
      out = out.slice(nl + 1);
    } else {
      out = out.slice(cut);
    }
  }
  return { output: out, numLinesAbove: existingAbove + extra };
}

function parseStepArgsWeb(s) {
  const args = {};
  if (s?.generic?.args && typeof s.generic.args === "object") {
    for (const [k, v] of Object.entries(s.generic.args)) {
      if (typeof v === "string") {
        try { args[k] = JSON.parse(v); } catch { args[k] = v; }
      } else {
        args[k] = v;
      }
    }
  }
  const rawJson = s?.metadata?.toolCall?.argumentsJson || s?.toolCall?.argumentsJson || "";
  if (typeof rawJson === "string" && rawJson.trim().startsWith("{")) {
    try {
      const parsed = JSON.parse(rawJson);
      if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
        for (const [k, v] of Object.entries(parsed)) {
          if (!(k in args)) args[k] = v;
        }
      }
    } catch {}
  }
  return args;
}

function argStrWeb(args, ...keys) {
  for (const k of keys) {
    const v = args?.[k];
    if (typeof v === "string" && v.trim()) return v.trim();
  }
  return "";
}

function argIntWeb(args, ...keys) {
  for (const k of keys) {
    const n = Number(args?.[k]);
    if (Number.isFinite(n) && n > 0) return Math.floor(n);
  }
  return 0;
}

function parseDesktopToolStepWeb(s, idx) {
  const type = s?.type || "";
  if (type === "CORTEX_STEP_TYPE_CHECKPOINT" || type === "CORTEX_STEP_TYPE_EPHEMERAL_MESSAGE") {
    return null;
  }
  const ri = s?.generic?.result?.stepRenderInfo || null;
  if (ri?.hidden) return null;

  const toolName = extractToolNameFromRawStep(s) || type.replace("CORTEX_STEP_TYPE_", "").toLowerCase();
  const lower = toolName.toLowerCase();
  const stRaw = s?.status || "";
  const isRunning = stRaw === "CORTEX_STEP_STATUS_RUNNING" || stRaw === "CORTEX_STEP_STATUS_PENDING" || stRaw === "CORTEX_STEP_STATUS_GENERATING";
  const status = s?.runCommand?.userRejected
    ? "rejected"
    : isRunning
      ? "running"
      : stRaw === "CORTEX_STEP_STATUS_WAITING"
        ? "waiting"
        : stRaw === "CORTEX_STEP_STATUS_CANCELED"
          ? "canceled"
          : stRaw === "CORTEX_STEP_STATUS_INTERRUPTED"
            ? "interrupted"
            : (stRaw === "CORTEX_STEP_STATUS_ERROR" || s?.error?.shortError || s?.errorMessage?.error?.shortError)
              ? "error"
              : "done";

  const args = parseStepArgsWeb(s);
  const metaAction = (s?.metadata?.toolAction || s?.toolCall?.toolAction || argStrWeb(args, "toolAction") || "").trim();
  const metaSummary = (s?.metadata?.toolSummary || s?.toolCall?.toolSummary || argStrWeb(args, "toolSummary") || "").trim();
  const metaLabel = isRunning ? (metaAction || metaSummary) : (metaSummary || metaAction);
  const errorText = (s?.errorMessage?.error?.userErrorMessage || s?.errorMessage?.error?.shortError || s?.error?.userErrorMessage || s?.error?.shortError || "").trim();

  const item = {
    stepIndex: idx,
    name: toolName,
    status,
    kind: "other",
    prefix: "",
    title: "",
    fileUri: "",
    fileName: "",
    startLine: 0,
    endLine: 0,
    additions: 0,
    deletions: 0,
    hasDiffStats: false,
    numResults: 0,
    command: "",
    cwd: "",
    output: "",
    numLinesAbove: 0,
    exitCode: null,
    markdown: "",
    errorText,
    isArtifact: false,
    raw: s
  };

  if (ri) {
    let prefix = String(ri.titlePrefix || "").trim();
    if (status === "rejected") prefix = "Rejected";
    else if (status === "canceled") prefix = "Canceled";
    else if (status === "interrupted") prefix = "Interrupted";
    else if (status === "done" && prefix.toLowerCase() === "running") prefix = "Ran";
    item.prefix = prefix;

    const textParts = [];
    let hasFileDiff = false;
    let hasFileRef = false;
    for (const tp of (ri.titleParts || [])) {
      if (tp.text && String(tp.text).trim()) textParts.push(String(tp.text).trim());
      if (tp.file?.uri) {
        hasFileRef = true;
        if (!item.fileUri) {
          item.fileUri = normalizeFileUriWeb(tp.file.uri);
          item.fileName = basenameOfUriWeb(tp.file.uri);
          item.startLine = Number(tp.file.startLine) || 0;
          item.endLine = Number(tp.file.endLine) || 0;
          item.isArtifact = /\/brain\/|\.gemini\/antigravity\/brain/i.test(item.fileUri);
        } else {
          textParts.push(basenameOfUriWeb(tp.file.uri) + lineRangeSuffixWeb(tp.file.startLine, tp.file.endLine));
        }
      }
      if (tp.fileDiff?.uri) {
        hasFileDiff = true;
        if (!item.fileUri) {
          item.fileUri = normalizeFileUriWeb(tp.fileDiff.uri);
          item.fileName = basenameOfUriWeb(tp.fileDiff.uri);
          item.isArtifact = /\/brain\/|\.gemini\/antigravity\/brain/i.test(item.fileUri);
        }
        if (tp.fileDiff.diffStats) {
          item.additions = Number(tp.fileDiff.diffStats.additions) || 0;
          item.deletions = Number(tp.fileDiff.diffStats.deletions) || 0;
          item.hasDiffStats = true;
        }
      }
      if (tp.badge?.label) textParts.push(String(tp.badge.label).trim());
    }
    item.title = textParts.length > 0 ? textParts.join(" ") : String(ri.title || "").trim();

    let hasTerminal = false;
    const mdParts = [];
    for (const cb of (ri.contentBlocks || [])) {
      if (cb.terminal) {
        hasTerminal = true;
        if (!item.command && cb.terminal.command) item.command = String(cb.terminal.command).trim();
        if (!item.cwd && cb.terminal.cwd) item.cwd = basenameOfUriWeb(cb.terminal.cwd);
        if (cb.terminal.exitCode !== undefined && cb.terminal.exitCode !== null) item.exitCode = Number(cb.terminal.exitCode);
        if (cb.terminal.output || cb.terminal.numLinesAbove) {
          const t = truncateTailOutputWeb(cb.terminal.output || "", Number(cb.terminal.numLinesAbove) || 0);
          item.output = t.output;
          item.numLinesAbove = t.numLinesAbove;
        }
      }
      if (cb.markdown && String(cb.markdown).trim()) mdParts.push(String(cb.markdown).trim());
    }
    if (mdParts.length > 0) item.markdown = mdParts.join("\n\n").slice(0, 4096);
    else if (ri.markdown && String(ri.markdown).trim()) item.markdown = String(ri.markdown).trim().slice(0, 4096);

    const agg = String(ri.aggregationKey || "").toLowerCase();
    if (hasFileDiff || agg === "edits" || ["edit_file", "write_file", "write_to_file", "replace_file_content", "multi_replace_file_content", "delete_file"].includes(lower)) {
      item.kind = "edit";
    } else if (hasTerminal || agg === "terminal" || lower === "run_command" || lower === "shell_command") {
      item.kind = "terminal";
      if (!item.command) item.command = (s?.runCommand?.commandLine || argStrWeb(args, "CommandLine", "commandLine", "command") || "").trim();
      if (!item.cwd) item.cwd = basenameOfUriWeb(s?.runCommand?.cwd || argStrWeb(args, "Cwd", "cwd"));
      if (isGenericCmdNoiseWeb(item.title)) item.title = isGenericCmdNoiseWeb(metaLabel) ? "" : metaLabel;
    } else if (agg === "folders" || lower === "list_dir" || lower === "list_directory") {
      item.kind = "list";
    } else if (hasFileRef || agg === "files" || lower === "read_file" || lower === "view_file") {
      item.kind = "read";
    } else if (/search|grep|find/i.test(lower)) {
      item.kind = "search";
    } else if (lower.startsWith("mcp_") || lower === "call_mcp_tool" || lower === "mcp_tool") {
      item.kind = "mcp";
    }
    if (!item.title && !item.fileName && !item.command && metaLabel) item.title = metaLabel;
    return item;
  }

  if (s?.runCommand || type === "CORTEX_STEP_TYPE_RUN_COMMAND" || ["run_command", "shell_command", "command", "command_status"].includes(lower)) {
    item.kind = "terminal";
    const rc = s?.runCommand || {};
    item.command = (rc.commandLine || rc.proposedCommandLine || argStrWeb(args, "CommandLine", "commandLine", "command") || s?.taskDetails?.description || "").trim();
    item.cwd = basenameOfUriWeb(rc.cwd || argStrWeb(args, "Cwd", "cwd"));
    if (rc.exitCode !== undefined && rc.exitCode !== null) item.exitCode = Number(rc.exitCode);
    const outObj = rc.combinedOutput || rc.stdoutOutput || rc.stderrOutput;
    const rawOut = outObj ? (outObj.full || outObj.truncated || "") : (rc.stdoutBuffer || rc.stderrBuffer || "");
    const rawAbove = outObj ? (Number(outObj.numLinesAbove) || 0) : (Number(rc.stdoutLinesAbove || rc.stderrLinesAbove) || 0);
    if (rawOut || rawAbove > 0) {
      const t = truncateTailOutputWeb(rawOut, rawAbove);
      item.output = t.output;
      item.numLinesAbove = t.numLinesAbove;
    }
    if (!isGenericCmdNoiseWeb(metaLabel)) item.title = metaLabel;
    item.prefix = status === "rejected" ? "Rejected"
      : status === "canceled" ? "Canceled"
      : status === "interrupted" ? "Interrupted"
      : status === "error" ? "Errored"
      : status === "waiting" ? "Run"
      : status === "running" ? "Running" : "Ran";
    if (item.title && item.title.toLowerCase().startsWith(item.prefix.toLowerCase() + " ")) {
      item.prefix = "";
    }
    return item;
  }

  if (s?.viewFile || type === "CORTEX_STEP_TYPE_VIEW_FILE" || lower === "view_file" || lower === "read_file") {
    item.kind = "read";
    item.prefix = isRunning ? "Analyzing" : "Analyzed";
    const rawPath = s?.viewFile?.absolutePathUri || argStrWeb(args, "Path", "AbsolutePath", "path", "absolutePathUri", "TargetFile");
    if (rawPath) {
      item.fileUri = normalizeFileUriWeb(rawPath);
      item.fileName = basenameOfUriWeb(rawPath);
      item.startLine = Number(s?.viewFile?.startLine) || argIntWeb(args, "StartLine", "start_line", "startLine");
      item.endLine = Number(s?.viewFile?.endLine) || argIntWeb(args, "EndLine", "end_line", "endLine");
      item.isArtifact = /\/brain\/|\.gemini\/antigravity\/brain/i.test(item.fileUri);
    } else if (metaLabel) {
      item.title = metaLabel;
    }
    return item;
  }

  if (s?.codeAction || type === "CORTEX_STEP_TYPE_CODE_ACTION" || ["edit_file", "write_file", "write_to_file", "replace_file_content", "multi_replace_file_content", "create_file", "delete_file"].includes(lower)) {
    item.kind = "edit";
    const ca = s?.codeAction || {};
    let isCreate = false;
    let isDelete = lower === "delete_file";
    let rawPath = ca.actionResult?.edit?.absoluteUri || ca.actionResult?.absoluteUri || "";
    if (ca.actionResult?.edit?.createFile) isCreate = true;
    if (ca.actionSpec?.deleteFile) {
      isDelete = true;
      rawPath = rawPath || ca.actionSpec.deleteFile.path?.absoluteUri || ca.actionSpec.deleteFile.path?.absoluteURI || "";
    } else if (ca.actionSpec?.createFile) {
      if (!ca.actionSpec.createFile.overwrite) isCreate = true;
      rawPath = rawPath || ca.actionSpec.createFile.path?.absoluteUri || ca.actionSpec.createFile.path?.absoluteURI || "";
    } else if (ca.actionSpec?.editFile) {
      rawPath = rawPath || ca.actionSpec.editFile.path?.absoluteUri || ca.actionSpec.editFile.path?.absoluteURI || "";
    }
    if (!rawPath) rawPath = argStrWeb(args, "Path", "TargetFile", "targetFile", "AbsolutePath", "path");
    if ((lower === "write_file" || lower === "write_to_file" || lower === "create_file") && !args.Overwrite && !args.overwrite) {
      isCreate = true;
    }
    item.prefix = isDelete ? (isRunning ? "Deleting" : "Deleted")
      : isCreate ? (isRunning ? "Creating" : "Created")
      : (isRunning ? "Editing" : "Edited");
    if (ca.diffStats) {
      item.additions = Number(ca.diffStats.additions) || 0;
      item.deletions = Number(ca.diffStats.deletions) || 0;
      item.hasDiffStats = true;
    }
    if (rawPath) {
      item.fileUri = normalizeFileUriWeb(rawPath);
      item.fileName = basenameOfUriWeb(rawPath);
      item.isArtifact = Boolean(ca.isArtifactFile) || /\/brain\/|\.gemini\/antigravity\/brain/i.test(item.fileUri);
    } else if (metaLabel) {
      item.title = metaLabel;
    }
    return item;
  }

  if (s?.listDirectory || type === "CORTEX_STEP_TYPE_LIST_DIRECTORY" || lower === "list_dir" || lower === "list_directory") {
    item.kind = "list";
    item.prefix = isRunning ? "Analyzing" : "Analyzed";
    const rawPath = s?.listDirectory?.directoryPathUri || argStrWeb(args, "DirectoryPath", "Path", "directoryPathUri", "path");
    if (rawPath) {
      item.fileUri = normalizeFileUriWeb(rawPath);
      item.fileName = basenameOfUriWeb(rawPath);
    } else if (metaLabel) {
      item.title = metaLabel;
    }
    return item;
  }

  if (s?.grepSearch || s?.codeSearch || s?.find || s?.searchWeb || /search|grep|find/i.test(lower)) {
    item.kind = "search";
    item.prefix = isRunning ? "Searching" : "Searched";
    if (s?.grepSearch) {
      item.title = (s.grepSearch.query || "").trim();
      item.numResults = Number(s.grepSearch.totalResults) || 0;
    } else if (s?.codeSearch) {
      item.title = (s.codeSearch.query || "").trim();
    } else if (s?.find) {
      item.title = (s.find.pattern || "").trim();
      item.numResults = Number(s.find.totalResults) || 0;
    } else if (s?.searchWeb) {
      const q = (s.searchWeb.query || "").trim();
      const d = (s.searchWeb.domain || "").trim();
      item.title = d && q ? `${d} for "${q}"` : q ? `web for "${q}"` : "";
    } else {
      item.title = argStrWeb(args, "Query", "query", "Pattern", "pattern") || metaLabel;
    }
    return item;
  }

  if (s?.mcpTool || type === "CORTEX_STEP_TYPE_MCP_TOOL" || lower === "call_mcp_tool" || lower === "mcp_tool") {
    item.kind = "mcp";
    item.prefix = (s?.mcpTool?.serverName || argStrWeb(args, "ServerName", "serverName") || "MCP").trim();
    item.title = (s?.mcpTool?.toolName || argStrWeb(args, "ToolName", "toolName") || metaLabel).trim();
    return item;
  }

  item.title = metaLabel || (s?.taskBoundary?.taskName || "").trim();
  return item;
}

function formatDesktopCompletedSummaryWeb(item) {
  const lang = getDesktopUiLangWeb();
  const steps = item?.steps || [];
  const toolSteps = steps.filter(s => s.kind !== "thinking");
  if (toolSteps.length === 0) {
    const thinkSteps = steps.filter(s => s.kind === "thinking");
    const lastDur = [...thinkSteps].reverse().find(s => s.duration)?.duration || item?.thinkingDuration || "";
    if (thinkSteps.length > 0 || (item?.thinking || "").trim()) {
      return formatThinkingTitleWeb(lastDur, false);
    }
    return lang === "zh" ? "已完成" : (lang === "ja" ? "完了" : "Done");
  }

  let exploredCount = 0;
  const exploredFiles = new Set();
  let exploredFileCount = 0;
  let exploredArtifactCount = 0;
  let terminalCount = 0;
  let editCount = 0;
  const editFiles = new Set();
  let editFileCount = 0;
  let editArtifactCount = 0;

  for (const s of toolSteps) {
    if (s.status === "error") continue;
    if (s.kind === "terminal") {
      terminalCount++;
    } else if (s.kind === "edit") {
      editCount++;
      const k = (s.fileUri || s.fileName || "").trim();
      if (k) {
        if (!editFiles.has(k)) {
          editFiles.add(k);
          if (s.isArtifact) editArtifactCount++; else editFileCount++;
        }
      } else {
        editFileCount++;
      }
    } else {
      exploredCount++;
      if (s.kind === "read" || s.kind === "list") {
        const k = (s.fileUri || s.fileName || "").trim();
        if (k && !exploredFiles.has(k)) {
          exploredFiles.add(k);
          if (s.isArtifact) exploredArtifactCount++; else exploredFileCount++;
        }
      }
    }
  }

  const noun = (files, arts) => {
    if (lang === "zh") return files > 0 ? "文件" : (arts > 0 ? "文档" : "");
    if (lang === "ja") return files > 0 ? "ファイル" : (arts > 0 ? "ドキュメント" : "");
    if (files > 0) return (files + arts) === 1 ? "file" : "files";
    if (arts > 0) return arts === 1 ? "artifact" : "artifacts";
    return "";
  };

  const parts = [];
  if (exploredCount > 0) {
    const n = noun(exploredFileCount, exploredArtifactCount);
    if (lang === "zh") parts.push(n ? `已浏览${n}` : "已探索");
    else if (lang === "ja") parts.push(n ? `${n}を調査` : "調査済み");
    else parts.push(n ? `Explored ${n}` : "Explored");
  }
  if (terminalCount > 0) {
    if (lang === "zh") parts.push(parts.length === 0 ? "已运行命令" : "运行命令");
    else if (lang === "ja") parts.push("コマンドを実行");
    else parts.push(`${parts.length === 0 ? "Ran" : "ran"} ${terminalCount === 1 ? "command" : "commands"}`);
  }
  if (editCount > 0) {
    const n = noun(editFileCount, editArtifactCount);
    if (lang === "zh") {
      const v = parts.length === 0 ? "已编辑" : "编辑";
      parts.push(n ? `${v}${n}` : `${v}文件`);
    } else if (lang === "ja") {
      parts.push(n ? `${n}を編集` : "ファイルを編集");
    } else {
      const v = parts.length === 0 ? "Edited" : "edited";
      parts.push(n ? `${v} ${n}` : `${v} files`);
    }
  }

  if (parts.length > 0) {
    return parts.join(lang === "zh" ? "、" : (lang === "ja" ? "・" : ", "));
  }
  if (lang === "zh") return `已完成 ${toolSteps.length} 项操作`;
  if (lang === "ja") return `${toolSteps.length} 件の操作を完了`;
  return toolSteps.length === 1 ? "Completed 1 action" : `Completed ${toolSteps.length} actions`;
}

function buildDesktopTimelineHtml(item) {
  const lang = getDesktopUiLangWeb();
  const steps = item?.steps || [];
  const hasTools = steps.some(s => s.kind !== "thinking");
  const chevronSvg = `<svg class="desktop-step-chevron" width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polyline points="9 6 15 12 9 18"></polyline></svg>`;
  const docSvg = `<svg class="desktop-file-icon" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline></svg>`;
  const folderSvg = `<svg class="desktop-file-icon" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"></path></svg>`;
  const copySvg = `<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg>`;

  const rows = steps.map((s, idx) => {
    if (s.kind === "thinking") {
      const title = formatThinkingTitleWeb(s.duration, s.isRunning);
      const stepKey = `think-${s.stepIndex ?? idx}`;
      const openAttr = !hasTools ? " open" : "";
      const shimmerCls = s.isRunning ? " is-running" : "";
      return `
        <details class="desktop-thinking-row" data-step-key="${escapeHtml(stepKey)}"${openAttr}>
          <summary class="desktop-step-summary${shimmerCls}">
            <span class="desktop-step-text">${escapeHtml(title)}</span>
            ${chevronSvg}
          </summary>
          <div class="desktop-thinking-body" data-i18n-skip="1">${escapeHtml(s.text || "")}</div>
        </details>
      `;
    }

    const prefix = localizeDesktopPrefixWeb(s.prefix);
    const isRunning = s.status === "running";
    const isError = s.status === "error" || (s.exitCode !== null && s.exitCode !== undefined && s.exitCode !== 0);
    const hasTerminal = Boolean(s.command || s.output || s.numLinesAbove > 0);
    const hasMarkdown = Boolean(s.markdown);
    const hasErrorBody = Boolean(s.errorText);
    const isExpandable = hasTerminal || hasMarkdown || hasErrorBody;

    let bodyParts = "";
    if (prefix) {
      bodyParts += `<span class="desktop-step-prefix">${escapeHtml(prefix)}</span>`;
    }
    if (s.fileName) {
      const refText = s.fileName + lineRangeSuffixWeb(s.startLine, s.endLine);
      const icon = s.kind === "list" ? folderSvg : docSvg;
      if (!isExpandable && s.fileUri) {
        bodyParts += `<button type="button" class="desktop-file-link" data-action="open-desktop-file" data-uri="${escapeHtml(s.fileUri)}" data-title="${escapeHtml(s.fileName)}">${icon}<span>${escapeHtml(refText)}</span></button>`;
      } else {
        bodyParts += `<span class="desktop-file-pill">${icon}<span>${escapeHtml(refText)}</span></span>`;
      }
      if (s.hasDiffStats || s.additions > 0 || s.deletions > 0) {
        bodyParts += `<span class="desktop-diff-stats"><span class="desktop-diff-add">+${s.additions || 0}</span><span class="desktop-diff-del">-${s.deletions || 0}</span></span>`;
      }
    } else if (s.title) {
      bodyParts += `<span class="desktop-step-text" data-i18n-skip="1">${escapeHtml(s.title)}</span>`;
    } else if (s.command) {
      bodyParts += `<span class="desktop-step-cmd-inline" data-i18n-skip="1">${escapeHtml(s.command)}</span>`;
    } else if (!prefix) {
      bodyParts += `<span class="desktop-step-text">${escapeHtml(formatToolName(s.name))}</span>`;
    }

    if (s.kind === "search" && s.numResults > 0) {
      const badge = lang === "zh" ? `${s.numResults} 条结果` : (lang === "ja" ? `${s.numResults} 件` : (s.numResults === 1 ? "1 result" : `${s.numResults} results`));
      bodyParts += `<span class="desktop-results-pill">${escapeHtml(badge)}</span>`;
    }
    if (isError) {
      bodyParts += `<span class="desktop-error-dot" aria-label="Error"></span>`;
    }

    const shimmerCls = isRunning ? " is-running" : "";

    if (!isExpandable) {
      return `<div class="desktop-step-row${shimmerCls}">${bodyParts}</div>`;
    }

    let detailBlocks = "";
    if (hasTerminal) {
      const cwdPrefix = s.cwd ? `${escapeHtml(s.cwd)} $` : "$";
      const hiddenLine = s.numLinesAbove > 0
        ? `<div class="desktop-terminal-hidden">${escapeHtml(
            lang === "zh" ? `…（上方已省略 ${s.numLinesAbove} 行）`
              : lang === "ja" ? `…（上部の ${s.numLinesAbove} 行を省略）`
              : `… (${s.numLinesAbove} ${s.numLinesAbove === 1 ? "line" : "lines"} above hidden)`
          )}</div>`
        : "";
      const cmdRow = s.command
        ? `
          <div class="desktop-terminal-cmd-row">
            <span class="desktop-terminal-prompt">${cwdPrefix}</span>
            <code class="desktop-terminal-cmd" data-i18n-skip="1">${escapeHtml(s.command)}</code>
            <button type="button" class="desktop-terminal-copy" data-action="copy-terminal-cmd" data-cmd="${escapeHtml(s.command)}" aria-label="Copy command">${copySvg}</button>
          </div>
        `
        : "";
      const outRow = (s.output || s.numLinesAbove > 0)
        ? `<pre class="desktop-terminal-output" data-i18n-skip="1">${hiddenLine}${escapeHtml(s.output || "")}</pre>`
        : "";
      detailBlocks += `<div class="desktop-terminal-card">${cmdRow}${outRow}</div>`;
    }
    if (hasMarkdown) {
      detailBlocks += `<div class="desktop-step-markdown" data-i18n-skip="1">${escapeHtml(s.markdown)}</div>`;
    }
    if (hasErrorBody) {
      detailBlocks += `<div class="desktop-step-error-box" data-i18n-skip="1">${escapeHtml(s.errorText)}</div>`;
    }

    const stepKey = `step-${s.stepIndex ?? idx}`;
    return `
      <details class="desktop-step-expandable" data-step-key="${escapeHtml(stepKey)}">
        <summary class="desktop-step-summary${shimmerCls}">
          ${bodyParts}
          ${chevronSvg}
        </summary>
        <div class="desktop-step-detail-body">
          ${detailBlocks}
        </div>
      </details>
    `;
  }).join("");

  return `<div class="desktop-step-timeline">${rows}</div>`;
}

function groupSteps(steps) {
  if (!steps || !steps.length) return [];

  const items = [];
  let currentBatch = null;
  let pendingFocus = "";
  let pendingToolsStartedAt = "";
  let turnPlan = null;
  let turnPlanAnchorIdx = -1;
  let turnStartItemIdx = 0;

  function flushBatch() {
    if (currentBatch && currentBatch.steps.length > 0) {
      if (pendingFocus) currentBatch.focusText = pendingFocus;
      if (pendingToolsStartedAt) {
        currentBatch.startedAt = pendingToolsStartedAt;
      }
      items.push(currentBatch);
      currentBatch = null;
      pendingToolsStartedAt = "";
    }
  }

  function insertTurnPlan() {
    if (!turnPlan || !turnPlan.items || turnPlan.items.length === 0) return;
    const planItem = {
      type: "step_plan",
      id: `item-step-plan-${turnPlan.anchorStepIndex}`,
      index: turnPlan.anchorStepIndex,
      stepPlan: turnPlan
    };
    let pos = items.length;
    while (pos > turnStartItemIdx && items[pos - 1].type === "agent") {
      pos--;
    }
    if (pos > turnStartItemIdx && items[pos - 1].type === "tools") {
      pos--;
    }
    items.splice(pos, 0, planItem);
    turnPlan = null;
    turnPlanAnchorIdx = -1;
  }

  for (let i = 0; i < steps.length; i++) {
    const s = steps[i];
    const type = s.type || "";
    const toolName = extractToolNameFromRawStep(s);

    if (toolName === "update_steps") {
      const parsedPlan = extractStepPlanFromRawStep(s, i);
      if (parsedPlan) {
        if (turnPlanAnchorIdx < 0) turnPlanAnchorIdx = i;
        parsedPlan.anchorStepIndex = turnPlanAnchorIdx;
        turnPlan = parsedPlan;
      }
      continue;
    }

    if (toolName === "update_focus") {
      const f = extractFocusFromRawStep(s);
      if (f) {
        pendingFocus = f;
        if (!pendingToolsStartedAt) pendingToolsStartedAt = stepCreatedTimestampWeb(s);
      }
      continue;
    }

    if (type === "CORTEX_STEP_TYPE_USER_INPUT") {
      flushBatch();
      insertTurnPlan();
      pendingFocus = "";
      pendingToolsStartedAt = "";
      const extracted = typeof extractUserInputTextAndSlashCommands === "function"
        ? extractUserInputTextAndSlashCommands(s.userInput)
        : { text: (s.userInput?.userResponse || s.userInput?.items?.[0]?.text || "").trim(), slashCommands: [] };
      const userText = extracted.text;
      const slashCommands = extracted.slashCommands;
      const hasMedia = Array.isArray(s.userInput?.media) && s.userInput.media.length > 0;
      const hasImages = Array.isArray(s.userInput?.images) && s.userInput.images.length > 0;
      const isArtifactApproval = Array.isArray(s.userInput?.artifactComments) && s.userInput.artifactComments.length > 0 && !userText;
      const isSystemApprovalText = userText.startsWith("Comments on artifact URI:") || userText.includes("The user has approved this document");

      if ((userText && !isArtifactApproval && !isSystemApprovalText) || hasMedia || hasImages) {
        const mediaList = [];
        const imageUrls = [];
        if (hasMedia) {
          for (const m of s.userInput.media) {
            if (m.uri) imageUrls.push(m.uri);
            if (m.thumbnail) mediaList.push(m.thumbnail);
            else if (m.inlineData) mediaList.push(m.inlineData);
          }
        }
        if (hasImages) {
          for (const img of s.userInput.images) {
            if (img.base64Data) mediaList.push(img.base64Data);
          }
        }
        items.push({
          type: "user",
          id: `item-user-${i}`,
          index: i,
          text: userText,
          slashCommands: slashCommands,
          media: mediaList,
          imageUrls: imageUrls,
          step: s
        });
      }
      turnStartItemIdx = items.length;
    } else if (type === "CORTEX_STEP_TYPE_PLANNER_RESPONSE") {
      const p = s.plannerResponse || {};
      const rawResp = (p.response || "").trim();
      const resp = cleanInternalSystemMessage(rawResp);
      const thinking = (p.rawThinking || p.thinking || "").trim();
      const stRaw = s.status || "";
      const isThinkRunning = (stRaw === "CORTEX_STEP_STATUS_RUNNING" || stRaw === "CORTEX_STEP_STATUS_GENERATING") && !resp && !p.thinkingDuration;

      if (thinking) {
        if (!pendingToolsStartedAt) pendingToolsStartedAt = stepCreatedTimestampWeb(s);
        if (!currentBatch) {
          currentBatch = {
            type: "tools",
            id: `item-tools-${i}`,
            index: i,
            startIndex: i,
            steps: [],
            toolNames: [],
            thinking: "",
            thinkingDuration: "",
            focusText: "",
            startedAt: ""
          };
        } else {
          currentBatch.index = i;
        }
        currentBatch.thinking = currentBatch.thinking ? `${currentBatch.thinking}\n\n${thinking}` : thinking;
        if (p.thinkingDuration) currentBatch.thinkingDuration = p.thinkingDuration;
        currentBatch.steps.push({
          kind: "thinking",
          stepIndex: i,
          name: "thinking",
          text: thinking,
          duration: p.thinkingDuration || "",
          isRunning: isThinkRunning,
          status: isThinkRunning ? "running" : "done",
          raw: s
        });
      }

      if (resp) {
        flushBatch();
        pendingFocus = "";
        pendingToolsStartedAt = "";
        const artList = s.artifacts ? [...s.artifacts] : [];
        items.push({
          type: "agent",
          id: `item-agent-${i}`,
          index: i,
          text: resp,
          thinking: "",
          artifacts: artList,
          step: s
        });
      }
    } else if (type === "CORTEX_STEP_TYPE_INVOKE_SUBAGENT") {
      // 与桌面端一致：在调用位置内联展示子代理卡片；Antigravity 2.35 起连续并行启动折叠为单行可展开项
      const inv = s.invokeSubagent || {};
      const specs = Array.isArray(inv.subagents) ? inv.subagents : [];
      const validSubs = [];
      (Array.isArray(inv.results) ? inv.results : []).forEach((res, k) => {
        const conversationId = (res && res.conversationId || "").trim();
        if (!conversationId) return;
        const spec = specs[k] || {};
        validSubs.push({ conversationId, typeName: spec.typeName || "", role: spec.role || "", prompt: spec.initialPrompt || "" });
      });
      if (validSubs.length > 0) {
        flushBatch();
        for (const sub of validSubs) {
          const prev = items[items.length - 1];
          if (prev && prev.type === "subagent_group") {
            prev.subs.push(sub);
          } else if (prev && prev.type === "subagent") {
            items[items.length - 1] = {
              type: "subagent_group",
              id: `item-subagent-group-${prev.sub.conversationId}`,
              index: prev.index,
              subs: [prev.sub, sub],
              step: s
            };
          } else {
            items.push({
              type: "subagent",
              id: `item-subagent-${sub.conversationId}`,
              index: i,
              sub,
              step: s
            });
          }
        }
      }
    } else if (s.workflow || type === "CORTEX_STEP_TYPE_RUN_WORKFLOW" || (s.toolCall && s.toolCall.name === "run_workflow")) {
      flushBatch();
      let wfData = s.workflow;
      if (!wfData && s.toolCall) {
        let args = {};
        try { args = JSON.parse(s.toolCall.argumentsJson || "{}"); } catch (e) {}
        wfData = {
          description: args.description || args.Description || "动态多智能体工作流",
          script: args.script || args.Script || "",
          scriptPath: args.scriptPath || args.ScriptPath || "",
          status: s.status === "CORTEX_STEP_STATUS_RUNNING" ? "running" : "done"
        };
      }
      items.push({
        type: "workflow",
        id: `item-workflow-${i}`,
        index: i,
        workflow: wfData || {},
        step: s
      });
    } else if (type === "CORTEX_STEP_TYPE_ERROR_MESSAGE") {
      const isUserVisible = (() => {
        if (s.errorMessage) {
          if (s.errorMessage.shouldShowUser === true) return true;
          if (s.errorMessage.shouldShowModel === true) return false;
          const short = (s.errorMessage.shortError || "").toLowerCase();
          const userMsg = (s.errorMessage.userErrorMessage || "").toLowerCase();
          if (short.includes("stream was interrupted") || userMsg.includes("stream was interrupted") ||
              short.includes("model produced invalid output") || userMsg.includes("model produced invalid output")) {
            return false;
          }
          return true;
        }
        if (s.error) {
          const short = (s.error.message || s.error.detail || "").toLowerCase();
          if (short.includes("stream was interrupted") || short.includes("model produced invalid output")) {
            return false;
          }
          return true;
        }
        return false;
      })();
      if (!isUserVisible) {
        continue;
      }
      flushBatch();
      const errText = s.errorMessage?.userErrorMessage
        || s.errorMessage?.shortError
        || s.errorMessage?.message
        || s.error?.message
        || "执行遇到错误";
      const errLower = errText.toLowerCase();
      const isOverflow = errLower.includes("context length") ||
        errLower.includes("maximum context") ||
        errLower.includes("context window") ||
        errLower.includes("prompt is too long") ||
        errLower.includes("exceeded the context") ||
        errLower.includes("token limit exceeded") ||
        errLower.includes("payload size exceeds") ||
        errLower.includes("context_overflow");
      items.push({
        type: "error",
        id: `item-error-${i}`,
        index: i,
        text: errText,
        isOverflow: isOverflow,
        step: s
      });
    } else if (type === "CORTEX_STEP_TYPE_SYSTEM_MESSAGE") {
      const sysMsg = String(s.systemMessage?.message || s.systemMessage?.content || "").trim().toLowerCase();
      if (sysMsg.startsWith("above is the conversation history from the parent conversation")) {
        flushBatch();
        insertTurnPlan();
        pendingFocus = "";
        pendingToolsStartedAt = "";
        turnStartItemIdx = items.length;
      }
      continue;
    } else if (type.startsWith("CORTEX_STEP_TYPE_") && type !== "CORTEX_STEP_TYPE_SYSTEM_MESSAGE") {
      const parsed = parseDesktopToolStepWeb(s, i);
      if (!parsed) continue;

      if (!currentBatch) {
        if (!pendingToolsStartedAt) pendingToolsStartedAt = stepCreatedTimestampWeb(s);
        currentBatch = {
          type: "tools",
          id: `item-tools-${i}`,
          index: i,
          startIndex: i,
          steps: [],
          toolNames: [],
          thinking: "",
          thinkingDuration: "",
          focusText: "",
          startedAt: ""
        };
      } else {
        currentBatch.index = i;
      }

      currentBatch.steps.push(parsed);

      if (!currentBatch.toolNames.includes(parsed.name) && currentBatch.toolNames.length < 3) {
        currentBatch.toolNames.push(parsed.name);
      }
    }
  }

  flushBatch();
  insertTurnPlan();
  items._activeFocus = pendingFocus;
  items._activeFocusStartedAt = pendingToolsStartedAt || "";
  return items;
}

function getItemFingerprint(item, isRunning, isLastItem, isActivePlan = false) {
  if (!item) return "";
  if (item.type === "step_plan") {
    const sp = item.stepPlan || {};
    const statusSig = (sp.items || []).map(it => `${it.key}:${it.status}:${it.title || ""}`).join("|");
    return `sp:${sp.anchorStepIndex || 0}:${isActivePlan ? 1 : 0}:${statusSig}`;
  }
  if (item.type === "user") {
    const mediaLen = (item.media || []).length;
    const scKey = (item.slashCommands || []).map(c => c.name).join(",");
    return `u:${item.text.length}:${item.text.slice(-10)}:m${mediaLen}:sc${scKey}`;
  }
  if (item.type === "agent") {
    const thinkLen = (item.thinking || "").length;
    const textLen = (item.text || "").length;
    const textLast = (item.text || "").slice(-12);
    const artKey = (item.artifacts || []).map(a => a.uri).join(",");
    return `a:${thinkLen}:${textLen}:${textLast}:${artKey}`;
  }
  if (item.type === "tools") {
    const active = (isRunning && isLastItem) ? "1" : "0";
    const thinkLen = (item.thinking || "").length;
    const lastStep = item.steps[item.steps.length - 1];
    const stepSig = item.steps.map(s => `${s.kind}:${s.status}:${(s.output || "").length}:${(s.title || s.fileName || "").length}:${s.additions || 0}:${s.deletions || 0}`).join("|");
    return `t:${item.steps.length}:${thinkLen}:${item.focusText || ""}:${item.startedAt || ""}:${lastStep?.status || ""}:${active}:${stepSig}`;
  }
  if (item.type === "error") {
    return `e:${item.text.length}:${item.text.slice(-12)}`;
  }
  if (item.type === "subagent") {
    const live = SubagentManager.byId.get(item.sub.conversationId);
    return `s:${item.sub.conversationId}:${live ? live.status : ""}:${live ? live.stepCount || 0 : 0}`;
  }
  if (item.type === "subagent_group") {
    const parts = (item.subs || []).map((sub) => {
      const live = SubagentManager.byId.get(sub.conversationId);
      return `${sub.conversationId}:${live ? live.status : ""}:${live ? live.stepCount || 0 : 0}`;
    });
    return `sg:${parts.join("|")}`;
  }
  if (item.type === "workflow") {
    const wf = item.workflow || {};
    const phasesCount = (wf.phases || []).length;
    const saCount = (wf.phases || []).flatMap(p => p.subagents || []).length;
    return `wf:${wf.status || ""}:${phasesCount}:${saCount}:${wf.duration || ""}`;
  }
  return "";
}

function formatToolName(name) {
  if (!name) return typeof t === "function" ? t("工具操作") : "工具操作";
  const raw = String(name).trim();
  const lower = raw.toLowerCase();
  const map = {
    "run_command": "运行终端命令",
    "manage_task": "管理后台任务",
    "schedule": "定时调度",
    "shell_command": "运行命令",
    "command": "运行命令",
    "view_file": "查看文件",
    "write_to_file": "写入文件",
    "replace_file_content": "编辑文件",
    "edit_file": "编辑文件",
    "create_file": "创建文件",
    "delete_file": "删除文件",
    "read_file": "读取文件",
    "list_dir": "浏览目录",
    "list_directory": "浏览目录",
    "search_code": "搜索代码",
    "grep_search": "搜索代码",
    "file_search": "搜索文件",
    "find_by_name": "搜索文件",
    "search_web": "搜索网络",
    "read_url_content": "读取网页",
    "invoke_subagent": "调用子代理",
    "define_subagent": "定义子代理",
    "manage_subagents": "管理子代理",
    "send_message": "发送消息",
    "browser_subagent": "浏览器代理",
    "ask_question": "询问用户",
    "generate_image": "生成图片",
    "call_mcp_tool": "调用 MCP 工具",
    "list_resources": "列出 MCP 资源",
    "read_resource": "读取 MCP 资源",
    "thinking": "思考中",
    "tool_call": "工具操作",
    "action": "操作"
  };
  if (map[lower]) return typeof t === "function" ? t(map[lower]) : map[lower];
  if (lower.startsWith("mcp_")) return `MCP: ${raw.slice(4)}`;
  return raw;
}

function buildWorkflowCardHtml(workflow) {
  if (!workflow) return "";
  const phases = workflow.phases || [];
  const status = (workflow.status || "running").toLowerCase();
  const isRunning = status === "running";
  const isDone = status === "done" || status === "completed";
  const isError = status === "error" || status === "failed";

  const badgeColor = isRunning ? "#a855f7" : (isDone ? "#22c55e" : (isError ? "#ef4444" : "var(--text-secondary)"));
  const badgeText = isRunning ? "运行中" : (isDone ? "已完成" : (isError ? "执行异常" : "等待中"));

  let stepperHtml = "";
  if (phases.length > 0) {
    stepperHtml = `
      <div class="workflow-stepper">
        ${phases.map((ph, idx) => {
          const phStatus = (ph.status || "pending").toLowerCase();
          const phActive = phStatus === "running";
          const phDone = phStatus === "done" || phStatus === "completed";
          const icon = phDone ? "✓" : (phActive ? "●" : String(idx + 1));
          return `
            <div class="workflow-step-item ${phActive ? 'active' : ''} ${phDone ? 'done' : ''}">
              <span style="font-weight: 700;">${icon}</span>
              <span>${escapeHtml(ph.name || `阶段 ${idx + 1}`)}</span>
              ${ph.duration ? `<span style="font-size: 10px; opacity: 0.7;">(${escapeHtml(ph.duration)})</span>` : ''}
            </div>
          `;
        }).join('<span style="color: var(--border-color); opacity: 0.5;">›</span>')}
      </div>
    `;
  }

  let subagentsHtml = "";
  const allSubagents = phases.flatMap(p => p.subagents || []);
  if (allSubagents.length > 0) {
    subagentsHtml = `
      <div style="font-size: 11px; font-weight: 600; color: var(--text-secondary); margin-top: 4px;">并发智能体矩阵 (${allSubagents.length})</div>
      <div class="workflow-subagent-grid">
        ${allSubagents.map(sa => {
          const saName = sa.displayName || sa.role || sa.typeName || "智能体";
          const saStatus = sa.statusText || (sa.isRunning ? "执行中" : "完成");
          return `
            <div class="workflow-subagent-mini-card">
              <div style="display: flex; justify-content: space-between; align-items: center;">
                <span style="font-weight: 600; color: var(--text-primary);">${escapeHtml(saName)}</span>
                <span style="font-size: 10px; color: ${sa.isRunning ? '#a855f7' : '#22c55e'};">${escapeHtml(saStatus)}</span>
              </div>
              ${sa.prompt ? `<div style="font-size: 11px; color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">${escapeHtml(sa.prompt)}</div>` : ''}
            </div>
          `;
        }).join('')}
      </div>
    `;
  }

  let logsHtml = "";
  const allLogs = phases.flatMap(p => p.logs || []);
  if (allLogs.length > 0) {
    logsHtml = `
      <details style="margin-top: 6px;">
        <summary style="cursor: pointer; font-size: 11.5px; color: var(--text-secondary); font-weight: 500;">查看执行日志 (${allLogs.length} 条)</summary>
        <div class="workflow-logs-box" style="margin-top: 6px;">
          ${allLogs.map(l => escapeHtml(l)).join('\n')}
        </div>
      </details>
    `;
  }

  let errorHtml = "";
  if (workflow.error) {
    errorHtml = `
      <div style="background: rgba(239, 68, 68, 0.1); border: 1px solid rgba(239, 68, 68, 0.3); border-radius: 8px; padding: 8px 10px; font-size: 12px; color: #ef4444;">
        ⚠️ ${escapeHtml(workflow.error)}
      </div>
    `;
  }

  return `
    <div class="workflow-stage-card">
      <div class="workflow-stage-header">
        <div class="workflow-stage-title-wrap">
          <div class="workflow-stage-icon">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2v20M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6"></path></svg>
          </div>
          <div>
            <div class="workflow-stage-title">${escapeHtml(workflow.description || "Workflow")}</div>
            ${workflow.duration ? `<span style="font-size: 11px; color: var(--text-muted);">${escapeHtml(workflow.duration)}</span>` : ''}
          </div>
        </div>
        <div style="display: flex; align-items: center; gap: 8px;">
          <span class="badge" style="font-size: 11px; background: ${badgeColor}22; color: ${badgeColor}; border: 1px solid ${badgeColor}44;">${escapeHtml(badgeText)}</span>
          ${isRunning ? `
            <button type="button" class="btn-cancel-task" onclick="if (typeof cancelCurrentTask === 'function') cancelCurrentTask();" style="border: 1px solid rgba(239,68,68,0.4); background: rgba(239,68,68,0.1); color: #ef4444; border-radius: 6px; padding: 3px 8px; font-size: 11px; cursor: pointer;">终止</button>
          ` : ''}
        </div>
      </div>
      ${stepperHtml}
      ${subagentsHtml}
      ${logsHtml}
      ${errorHtml}
    </div>
  `;
}

function buildStepPlanStatusIconHtml(status, isRunning) {
  if (status === "done") {
    return `<span class="step-plan-icon done" aria-hidden="true"><svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9.5"></circle><path d="M8.5 12.4l2.3 2.3 4.8-5"></path></svg></span>`;
  }
  if (status === "failed") {
    return `<span class="step-plan-icon failed" aria-hidden="true"><svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9.5"></circle><line x1="12" y1="8" x2="12" y2="12.5"></line><line x1="12" y1="16" x2="12.01" y2="16"></line></svg></span>`;
  }
  if (status === "in_progress") {
    if (isRunning) {
      return `<span class="step-plan-icon in-progress" aria-hidden="true"><span class="step-plan-dot-spinner"><span></span><span></span><span></span><span></span></span></span>`;
    }
    return `<span class="step-plan-icon paused" aria-hidden="true"><svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9.5"></circle><line x1="10" y1="9" x2="10" y2="15"></line><line x1="14" y1="9" x2="14" y2="15"></line></svg></span>`;
  }
  if (status === "skipped") {
    return `<span class="step-plan-icon skipped" aria-hidden="true"><svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><circle cx="12" cy="12" r="9.5"></circle></svg></span>`;
  }
  return `<span class="step-plan-icon pending" aria-hidden="true"><svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><circle cx="12" cy="12" r="9.5"></circle></svg></span>`;
}

function buildStepPlanCardHtml(plan, isActivePlan) {
  if (!plan || !Array.isArray(plan.items) || plan.items.length === 0) return "";
  const listHtml = `
    <div class="step-plan-card">
      <ol class="step-plan-list">
        ${plan.items.map(it => {
          const st = it.status || "pending";
          return `
            <li class="step-plan-item status-${escapeHtml(st)}">
              ${buildStepPlanStatusIconHtml(st, isActivePlan)}
              <span class="step-plan-title">${escapeHtml(it.title || "")}</span>
            </li>
          `;
        }).join("")}
      </ol>
    </div>
  `;
  if (isActivePlan) {
    return listHtml;
  }
  const done = Number(plan.doneCount) || 0;
  const total = Number(plan.totalCount) || 0;
  const failed = Number(plan.failedCount) || 0;
  const skipped = Number(plan.skippedCount) || 0;
  const r = 4.5;
  const circ = 2 * Math.PI * r;
  const ratio = total > 0 ? Math.max(0, Math.min(1, done / total)) : 0;
  const parts = [];
  const isChinese = typeof isZh === "function" && isZh();
  const isJapanese = typeof isJa === "function" && isJa();
  if (total > 0) {
    parts.push(isChinese ? `${done}/${total} 步` : (isJapanese ? `${done}/${total} ステップ` : `${done}/${total} steps`));
  }
  if (failed > 0) {
    parts.push(isChinese ? `${failed} 失败` : (isJapanese ? `${failed} 失敗` : `${failed} failed`));
  }
  if (skipped > 0) {
    parts.push(isChinese ? `${skipped} 跳过` : (isJapanese ? `${skipped} スキップ` : `${skipped} skipped`));
  }
  const label = parts.join(" · ");
  return `
    <details class="step-plan-collapsible">
      <summary class="step-plan-chip">
        <svg class="step-plan-ring" width="13" height="13" viewBox="0 0 14 14" aria-hidden="true">
          <circle cx="7" cy="7" r="4.5" fill="none" stroke="currentColor" stroke-opacity="0.25" stroke-width="2"></circle>
          <circle cx="7" cy="7" r="4.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-dasharray="${(ratio * circ).toFixed(2)} ${circ.toFixed(2)}" transform="rotate(-90 7 7)"></circle>
        </svg>
        <span class="step-plan-chip-label">${escapeHtml(label)}</span>
        <svg class="step-plan-chip-chevron" width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 6 15 12 9 18"></polyline></svg>
      </summary>
      ${listHtml}
    </details>
  `;
}

function buildActiveFocusRowHtml(focusText, startedAt, thinkingHtml, stepItemsHtml, isExpandable) {
  const startMs = startedAt ? Date.parse(startedAt) : NaN;
  const timerHtml = Number.isFinite(startMs)
    ? `<span class="focus-elapsed-timer" data-started-at="${startMs}">${escapeHtml(formatElapsedDurationWeb(Date.now() - startMs))}</span>`
    : "";
  const hasDetails = isExpandable && (Boolean(thinkingHtml) || Boolean(stepItemsHtml));
  if (!hasDetails) {
    return `
      <div class="focus-tool-row static">
        <span class="focus-tool-text">${escapeHtml(focusText)}</span>
        ${timerHtml}
      </div>
    `;
  }
  return `
    <details class="focus-tool-row">
      <summary class="focus-tool-summary">
        <span class="focus-tool-text">${escapeHtml(focusText)}</span>
        ${timerHtml}
        <svg class="focus-tool-chevron" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 6 15 12 9 18"></polyline></svg>
      </summary>
      <div class="desktop-tool-expanded">
        ${thinkingHtml}
        ${stepItemsHtml}
      </div>
    </details>
  `;
}

function generateItemHtml(item, isRunning, isLastItem, isActivePlan = false) {
  if (item.type === "step_plan") {
    return buildStepPlanCardHtml(item.stepPlan, isActivePlan);
  }
  if (item.type === "error") {
    if (item.isOverflow || item.errorKind === "context_overflow") {
      return `
        <div class="rescue-bubble">
          <div class="rescue-bubble-header">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
            <span>会话上下文超出模型上限</span>
          </div>
          <div class="rescue-bubble-desc">
            当前会话历史累计已达模型物理窗口上限，无法继续接收新的上下文。建议立即压缩或新开对话。<br/>
            <span style="opacity:0.75;font-size:11px;">${escapeHtml(item.text)}</span>
          </div>
          <div class="rescue-bubble-actions">
            <button type="button" class="rescue-btn primary" onclick="sendCompactCommand()" style="display:inline-flex;align-items:center;justify-content:center;gap:5px;">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M7 5l5 5 5-5"/><path d="M7 19l5-5 5 5"/></svg>
              一键压缩 /compact
            </button>
            <button type="button" class="rescue-btn secondary" onclick="document.getElementById('btn-new')?.click()">开启新会话</button>
          </div>
        </div>
      `;
    }
    const attemptCount = item.attemptCount || 0;
    const maxAttempts = item.maxAttempts || 0;
    const badgeText = (attemptCount && maxAttempts)
      ? `ERROR · 尝试 ${attemptCount}/${maxAttempts}`
      : (attemptCount > 1 ? `ERROR · 重试 ${attemptCount} 次` : "error");
    const titleText = (attemptCount > 1)
      ? `执行遇到错误 (已重试 ${attemptCount} 次)`
      : "执行遇到错误";
    return `
      <div class="agent-error-card">
        <div class="agent-error-icon">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
            <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"></path>
            <line x1="12" y1="9" x2="12" y2="13"></line>
            <line x1="12" y1="17" x2="12.01" y2="17"></line>
          </svg>
        </div>
        <div class="agent-error-body">
          <div class="agent-error-header">
            <span class="badge badge-error">${escapeHtml(badgeText)}</span>
            <span class="agent-error-title">${escapeHtml(titleText)}</span>
          </div>
          <div class="agent-error-message">${escapeHtml(item.text)}</div>
        </div>
      </div>
    `;
  }
  if (item.type === "subagent") {
    return buildSubagentCardHtml(item.sub);
  }
  if (item.type === "subagent_group") {
    const subs = item.subs || [];
    let running = 0;
    let done = 0;
    for (const sub of subs) {
      const live = SubagentManager.byId.get(sub.conversationId);
      const st = live?.status || sub.status || "";
      if (st === "running") running++;
      else if (st !== "gone") done++;
    }
    const statusText = running > 0
      ? (done > 0 ? `${running} 运行中 · ${done} 已完成` : `${running} 运行中`)
      : "全部已完成";
    const iconHtml = running > 0
      ? '<div class="ios-spinner subagent-icon-spin"></div>'
      : '<span class="subagent-icon done">✓</span>';
    return `
      <details class="subagent-group-card">
        <summary class="subagent-card" style="list-style:none;cursor:pointer;">
          ${iconHtml}
          <span class="subagent-main">
            <span class="subagent-name">并行启动 ${subs.length} 个子代理</span>
            <span class="subagent-meta">${escapeHtml(statusText)}</span>
          </span>
          <span class="subagent-arrow">⌄</span>
        </summary>
        <div class="subagent-group-list" style="padding-left:10px;display:flex;flex-direction:column;gap:6px;margin-top:6px;">
          ${subs.map((sub) => buildSubagentCardHtml(sub)).join("")}
        </div>
      </details>
    `;
  }
  if (item.type === "workflow") {
    return buildWorkflowCardHtml(item.workflow);
  }
  if (item.type === "user") {
    let imagesHtml = "";
    if (item.imageUrls && item.imageUrls.length > 0) {
      imagesHtml = `<div class="user-message-images">` +
        item.imageUrls.map((u, idx) => {
          const rawUrl = resolveMediaRawUrl(u);
          const thumb = (item.media && item.media[idx]) ? (item.media[idx].startsWith("data:") ? item.media[idx] : `data:image/jpeg;base64,${item.media[idx]}`) : "";
          return buildImageThumbnailCard(rawUrl, thumb, "上传图片");
        }).join("") + `</div>`;
    } else if (item.media && item.media.length > 0) {
      imagesHtml = `<div class="user-message-images">` +
        item.media.map(m => {
          const src = m.startsWith("data:") ? m : `data:image/jpeg;base64,${m}`;
          return `<img src="${escapeHtml(src)}" class="bubble-image" data-action="open-image" alt="上传图片" />`;
        }).join("") + `</div>`;
    }
    const parsedUser = (typeof parseUserAttachmentBlock === "function" && item.text)
      ? parseUserAttachmentBlock(item.text)
      : { body: item.text || "", files: [] };
    const filesHtml = (typeof buildUserAttachmentCardsHtml === "function" && parsedUser.files.length > 0)
      ? buildUserAttachmentCardsHtml(parsedUser.files)
      : "";
    const renderedUserText = parsedUser.body
      ? (typeof renderUserTextWithSlashPills === "function"
          ? renderUserTextWithSlashPills(parsedUser.body, item.slashCommands)
          : escapeHtml(parsedUser.body))
      : "";
    const bubbleInner = `${imagesHtml}${renderedUserText ? `<div class="user-message-text" data-i18n-skip="1">${renderedUserText}</div>` : ""}`;
    if (!filesHtml) {
      return `<div class="bubble-col"><div class="bubble">${bubbleInner}</div>${buildMessageActionsHtml("user")}</div>`;
    }
    const bubbleHtml = bubbleInner ? `<div class="bubble">${bubbleInner}</div>` : "";
    return `<div class="bubble-col user-bubble-stack">${filesHtml}${bubbleHtml}${buildMessageActionsHtml("user")}</div>`;
  }

  if (item.type === "agent") {
    let thoughtHtml = "";
    if (item.thinking) {
      thoughtHtml = `
        <details class="thought-box">
          <summary>🧠 Agent 思考过程 (${item.thinking.length} 字符)</summary>
          <div class="thought-content" data-i18n-skip="1">${escapeHtml(item.thinking)}</div>
        </details>
      `;
    }
    const runningLabel = typeof t === "function" ? t("执行中...") : "执行中...";
    const bodyHtml = item.text ? getCachedMarkdown(item.text, { streaming: !!(isRunning && isLastItem) }) : `<span style="color:var(--text-muted);">${escapeHtml(runningLabel)}</span>`;
    let artifactsHtml = "";
    if (Array.isArray(item.artifacts) && item.artifacts.length > 0) {
      artifactsHtml = `<div class="message-artifact-cards">` +
        item.artifacts.map(art => {
          const title = escapeHtml(art.title || (typeof t === "function" ? t("文档详情") : "文档详情"));
          const summary = art.summary ? `<div class="artifact-card-summary" data-i18n-skip="1">${escapeHtml(art.summary)}</div>` : "";
          const uriAttr = escapeHtml(art.uri || "");
          return `
            <div class="artifact-preview-card" data-uri="${uriAttr}" data-title="${title}">
              <div class="artifact-card-header">
                <span class="artifact-card-icon">📄</span>
                <span class="artifact-card-title" data-i18n-skip="1">${title}</span>
              </div>
              ${summary}
            </div>
          `;
        }).join("") + `</div>`;
    }
    return `
      <div class="bubble-col">
        <div class="bubble markdown-body">
          ${thoughtHtml}
          <div class="agent-message-body" data-i18n-skip="1">${bodyHtml}</div>
          ${artifactsHtml}
        </div>
        ${buildMessageActionsHtml("agent")}
      </div>
    `;
  }

  if (item.type === "tools") {
    const isActive = isRunning && isLastItem;
    const toolOnlySteps = item.steps.filter(s => s.kind !== "thinking");
    const toolCount = toolOnlySteps.length;
    const timelineHtml = buildDesktopTimelineHtml(item);

    if (isActive) {
      const liveFocus = (item.focusText || "").trim() || "Working";
      return buildActiveFocusRowHtml(liveFocus, item.startedAt, "", timelineHtml, true);
    }

    if (window.CompactModePrefs?.shouldCollapseTools?.()) {
      const totalOps = Math.max(toolCount, item.steps.length);
      return `<div class="tool-batch-compact-line">📄 已完成 ${totalOps} 项检索与命令操作</div>`;
    }

    const summaryText = formatDesktopCompletedSummaryWeb(item);
    return `
      <details class="tool-batch-accordion desktop-completed-accordion">
        <summary class="focus-tool-summary">
          <span class="tool-batch-completed-text">${escapeHtml(summaryText)}</span>
          <svg class="focus-tool-chevron" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <polyline points="9 6 15 12 9 18"></polyline>
          </svg>
        </summary>
        <div class="desktop-tool-expanded">
          ${timelineHtml}
        </div>
      </details>
    `;
  }

  return "";
}

function patchAgentBodyHtml(bodyEl, bodyHtml) {
  const existingHydrated = bodyEl.querySelectorAll('.agent-embed-card[data-hydrated="true"]');
  if (!existingHydrated || existingHydrated.length === 0) {
    bodyEl.innerHTML = bodyHtml;
    return;
  }

  // Preserve live hydrated <agent-embed> cards in place so streaming token updates
  // never detach or reload mounted iframes.
  const temp = document.createElement("div");
  temp.innerHTML = bodyHtml;
  const incomingCards = Array.from(temp.querySelectorAll(".agent-embed-card"));
  if (incomingCards.length !== existingHydrated.length) {
    bodyEl.innerHTML = bodyHtml;
    return;
  }
  for (let i = 0; i < incomingCards.length; i++) {
    if (incomingCards[i].getAttribute("data-src") !== existingHydrated[i].getAttribute("data-src")) {
      bodyEl.innerHTML = bodyHtml;
      return;
    }
    // Only support non-destructive top-level preservation; fall back if nested cleanly via cache
    if (incomingCards[i].parentElement !== temp || existingHydrated[i].parentElement !== bodyEl) {
      bodyEl.innerHTML = bodyHtml;
      return;
    }
  }

  // Reconcile segments before/between/after each surviving hydrated card without detaching the card
  const tempNodes = Array.from(temp.childNodes);
  let cursor = bodyEl.firstChild;
  let cardIdx = 0;

  for (const node of tempNodes) {
    if (node.nodeType === 1 && node.classList.contains("agent-embed-card")) {
      const liveCard = existingHydrated[cardIdx++];
      while (cursor && cursor !== liveCard) {
        const next = cursor.nextSibling;
        bodyEl.removeChild(cursor);
        cursor = next;
      }
      if (cursor === liveCard) {
        cursor = liveCard.nextSibling;
      }
    } else {
      bodyEl.insertBefore(node, cursor);
    }
  }
  while (cursor) {
    const next = cursor.nextSibling;
    bodyEl.removeChild(cursor);
    cursor = next;
  }
}

function scheduleRenderMessages(steps, isRunning = false) {
  pendingRenderData = { steps, isRunning };
  if (!pendingRenderRaf) {
    pendingRenderRaf = requestAnimationFrame(() => {
      pendingRenderRaf = null;
      if (pendingRenderData) {
        renderMessages(pendingRenderData.steps, pendingRenderData.isRunning);
        pendingRenderData = null;
      }
    });
  }
}

function renderMessages(steps, isRunning = false) {
  const streamEl = document.getElementById("messages-stream");
  if (!streamEl) return;
  initScrollListener();

  const loadingEl = streamEl.querySelector(".loading-state");
  if (loadingEl) {
    loadingEl.remove();
  }

  if (!steps || steps.length === 0) {
    streamEl.innerHTML = '<div class="loading-state"><p>暂无消息</p></div>';
    updateContinueButton(false);
    return;
  }

  if (activeCascadeId) {
    setSessionStepsCache(activeCascadeId, { steps, isRunning });
  }

  const items = groupSteps(steps);
  const isLastError = checkLatestMessageIsError(items, isRunning, true);
  updateContinueButton(isLastError);

  const isSessionSwitch = streamEl.__currentCascadeId !== activeCascadeId;
  streamEl.__currentCascadeId = activeCascadeId;

  const lastItem = items[items.length - 1];
  const isAwaiting = isRunning && lastItem?.type !== "tools";
  let activePlanId = null;
  if (isRunning) {
    let lastUserIdx = -1;
    for (let i = items.length - 1; i >= 0; i--) {
      if (items[i].type === "user") { lastUserIdx = i; break; }
    }
    for (let i = items.length - 1; i > lastUserIdx; i--) {
      if (items[i].type === "agent") break;
      if (items[i].type === "step_plan") {
        activePlanId = items[i].id;
        break;
      }
    }
  }
  const activeFocusText = (items._activeFocus || "").trim();
  const activeFocusStartedAt = items._activeFocusStartedAt || "";
  const buildThinkingIndicatorHtml = () => {
    return buildActiveFocusRowHtml(activeFocusText || "Working", activeFocusStartedAt, "", "", false);
  };

  const preDist = streamEl.scrollHeight - streamEl.scrollTop - streamEl.clientHeight;
  const wasNearBottom = (userIsNearBottom || preDist <= 120) && !isUserTouching;

  let hasDOMChanges = false;
  const changedEls = [];

  if (isSessionSwitch) {
    // P2 / B-5 Fast Path: Session switch! Replace the entire child tree in one single
    // atomic DOM layout operation via DocumentFragment + replaceChildren to eliminate
    // tens of layout reflows and eliminate stuttering.
    const fragment = document.createDocumentFragment();
    for (let i = 0; i < items.length; i++) {
      const item = items[i];
      const isLastItem = (i === items.length - 1);
      const isActivePlan = item.type === "step_plan" && item.id === activePlanId;
      const fp = getItemFingerprint(item, isRunning, isLastItem, isActivePlan);

      let rowClass = "message-row";
      if (item.type === "user") {
        rowClass = "message-row user";
      } else if (item.type === "agent") {
        rowClass = "message-row agent";
      } else if (item.type === "tools") {
        rowClass = (isRunning && isLastItem) ? "message-row focus-indicator-row" : "message-row tool-batch-row";
      } else if (item.type === "error") {
        rowClass = "message-row agent error-row";
      } else if (item.type === "subagent") {
        rowClass = "message-row subagent-row";
      } else if (item.type === "workflow") {
        rowClass = "message-row workflow-row";
      } else if (item.type === "step_plan") {
        rowClass = "message-row step-plan-row";
      }

      const newEl = document.createElement("div");
      newEl.id = item.id;
      newEl.className = rowClass;
      newEl.setAttribute("data-fp", fp);
      if (item.index !== undefined) newEl.setAttribute("data-step-index", String(item.index));
      newEl.innerHTML = generateItemHtml(item, isRunning, isLastItem, isActivePlan);
      fragment.appendChild(newEl);
    }

    if (isAwaiting) {
      const thinkingIndicator = document.createElement("div");
      thinkingIndicator.id = "agent-thinking-indicator";
      thinkingIndicator.className = "message-row focus-indicator-row";
      thinkingIndicator.setAttribute("data-focus-sig", `${activeFocusText}|${activeFocusStartedAt}`);
      thinkingIndicator.innerHTML = buildThinkingIndicatorHtml();
      fragment.appendChild(thinkingIndicator);
    }

    streamEl.replaceChildren(fragment);
    hasDOMChanges = true;
  } else {
    // Incremental diffing for live streaming updates within the same active session
    const currentChildIds = new Set(items.map(it => it.id));
    currentChildIds.add("agent-thinking-indicator");

    // Remove nodes that no longer exist
    Array.from(streamEl.children).forEach(child => {
      if (!currentChildIds.has(child.id)) {
        child.remove();
        hasDOMChanges = true;
      }
    });

    for (let i = 0; i < items.length; i++) {
      const item = items[i];
      const isLastItem = (i === items.length - 1);
      const isActivePlan = item.type === "step_plan" && item.id === activePlanId;
      const fp = getItemFingerprint(item, isRunning, isLastItem, isActivePlan);

      let rowClass = "message-row";
      if (item.type === "user") {
        rowClass = "message-row user";
      } else if (item.type === "agent") {
        rowClass = "message-row agent";
      } else if (item.type === "tools") {
        rowClass = (isRunning && isLastItem) ? "message-row focus-indicator-row" : "message-row tool-batch-row";
      } else if (item.type === "error") {
        rowClass = "message-row agent error-row";
      } else if (item.type === "subagent") {
        rowClass = "message-row subagent-row";
      } else if (item.type === "workflow") {
        rowClass = "message-row workflow-row";
      } else if (item.type === "step_plan") {
        rowClass = "message-row step-plan-row";
      }

      let existingEl = document.getElementById(item.id);
      if (existingEl) {
        // Keep DOM child order aligned if step_plan moved above a newer active tools batch
        const expectedAtI = streamEl.children[i];
        if (expectedAtI !== existingEl) {
          streamEl.insertBefore(existingEl, expectedAtI || null);
          hasDOMChanges = true;
        }
        if (item.index !== undefined) existingEl.setAttribute("data-step-index", String(item.index));
        if (existingEl.getAttribute("data-fp") !== fp) {
          existingEl.setAttribute("data-fp", fp);
          existingEl.className = rowClass;

          // Optimization for agent streaming updates: in-place DOM patch
          const bubble = existingEl.querySelector(".bubble.markdown-body");
          const bodyEl = existingEl.querySelector(".agent-message-body");
          if (item.type === "agent" && bubble && bodyEl) {
            let thoughtDetails = bubble.querySelector(".thought-box");
            if (item.thinking) {
              if (thoughtDetails) {
                const summary = thoughtDetails.querySelector("summary");
                const content = thoughtDetails.querySelector(".thought-content");
                if (summary) summary.textContent = `🧠 Agent 思考过程 (${item.thinking.length} 字符)`;
                if (content && content.textContent !== item.thinking) content.textContent = item.thinking;
              } else {
                const temp = document.createElement("div");
                temp.innerHTML = `
                  <details class="thought-box">
                    <summary>🧠 Agent 思考过程 (${item.thinking.length} 字符)</summary>
                    <div class="thought-content">${escapeHtml(item.thinking)}</div>
                  </details>
                `;
                bubble.insertBefore(temp.firstElementChild, bodyEl);
              }
            } else if (thoughtDetails) {
              thoughtDetails.remove();
            }

            const bodyHtml = item.text ? getCachedMarkdown(item.text, { streaming: !!(isRunning && isLastItem) }) : '<span style="color:var(--text-muted);">执行中...</span>';
            patchAgentBodyHtml(bodyEl, bodyHtml);
          } else {
            const wasOpen = existingEl.querySelector(":scope > details")?.open ?? existingEl.querySelector("details")?.open;
            const openKeys = new Set();
            existingEl.querySelectorAll("details[data-step-key]").forEach(d => {
              if (d.open) openKeys.add(d.getAttribute("data-step-key"));
            });
            existingEl.innerHTML = generateItemHtml(item, isRunning, isLastItem, isActivePlan);
            if (wasOpen) {
              const newDetails = existingEl.querySelector(":scope > details") || existingEl.querySelector("details");
              if (newDetails) newDetails.open = true;
            }
            if (openKeys.size > 0) {
              existingEl.querySelectorAll("details[data-step-key]").forEach(d => {
                if (openKeys.has(d.getAttribute("data-step-key"))) d.open = true;
              });
            }
          }
          hasDOMChanges = true;
          changedEls.push(existingEl);
        }
      } else {
        const newEl = document.createElement("div");
        newEl.id = item.id;
        // Antigravity 2.33.0: avoid flashing/re-animating user messages or active turns on immediate send
        newEl.className = (item.type === "user" || (isRunning && item.type === "agent")) ? rowClass : (rowClass + " message-entering");
        newEl.setAttribute("data-fp", fp);
        if (item.index !== undefined) newEl.setAttribute("data-step-index", String(item.index));
        newEl.innerHTML = generateItemHtml(item, isRunning, isLastItem, isActivePlan);

        const expectedAtI = streamEl.children[i];
        const indicator = document.getElementById("agent-thinking-indicator");
        const refNode = expectedAtI || indicator || null;
        streamEl.insertBefore(newEl, refNode);
        hasDOMChanges = true;
        changedEls.push(newEl);
      }
    }

    let thinkingIndicator = document.getElementById("agent-thinking-indicator");
    const focusSig = `${activeFocusText}|${activeFocusStartedAt}`;
    if (isAwaiting) {
      if (!thinkingIndicator) {
        thinkingIndicator = document.createElement("div");
        thinkingIndicator.id = "agent-thinking-indicator";
        thinkingIndicator.className = "message-row focus-indicator-row";
        thinkingIndicator.setAttribute("data-focus-sig", focusSig);
        thinkingIndicator.innerHTML = buildThinkingIndicatorHtml();
        streamEl.appendChild(thinkingIndicator);
        hasDOMChanges = true;
      } else if (thinkingIndicator.getAttribute("data-focus-sig") !== focusSig) {
        thinkingIndicator.className = "message-row focus-indicator-row";
        thinkingIndicator.setAttribute("data-focus-sig", focusSig);
        thinkingIndicator.innerHTML = buildThinkingIndicatorHtml();
        hasDOMChanges = true;
      }
    } else if (thinkingIndicator) {
      thinkingIndicator.remove();
      hasDOMChanges = true;
    }
  }
  syncFocusElapsedTimers();

  // Hydrate Mermaid, <agent-embed> & PR pills only within changed nodes (not the whole streamEl)
  if (hasDOMChanges) {
    const hydrateRoots = isSessionSwitch ? [streamEl] : changedEls;
    for (const root of hydrateRoots) {
      renderAllMermaidDiagrams(root);
      if (typeof hydrateAllAgentEmbeds === "function") {
        hydrateAllAgentEmbeds(root);
      }
      window.V233?.hydrateChatPRPills?.(root);
    }
  }
  window.V233?.applyBranchDecorations?.(streamEl, activeCascadeId);

  // 1. First-time render on entering a conversation: align cleanly to bottom without whole-page jump
  if (!hasInitiallyAligned) {
    hasInitiallyAligned = true;
    prevWasRunning = isRunning;
    userIsNearBottom = true;
    streamEl.scrollTop = streamEl.scrollHeight;
    return;
  }

  // 2. Active task finished: preserve position stably without upward jumping
  const justFinished = (prevWasRunning && !isRunning);
  prevWasRunning = isRunning;

  if (justFinished) {
    LocalQueueManager.onAgentCompleted();
    if (typeof refreshActionAvailability === "function") refreshActionAvailability();
    if (wasNearBottom) {
      userIsNearBottom = true;
      streamEl.scrollTop = streamEl.scrollHeight;
    }
  }

  // 3. Live streaming while running: pin to bottom ONLY if user was near bottom and not actively dragging
  if (isRunning && hasDOMChanges && wasNearBottom) {
    userIsNearBottom = true;
    streamEl.scrollTop = streamEl.scrollHeight;
  }

  clearTimeout(renderMessages._stepperTimer);
  renderMessages._stepperTimer = setTimeout(() => updatePromptJumpStepper(), 50);
}

// --- Running Background Tasks Manager (Desktop Antigravity Parity) ---
const RunningTasksManager = {
  cascadeId: null,
  tasks: [],
  isExpanded: true,

  init(cascadeId) {
    if (!cascadeId) {
      this.cascadeId = null;
      this.tasks = [];
      this.render();
      return;
    }
    if (this.cascadeId !== cascadeId) {
      this.cascadeId = cascadeId;
      this.tasks = [];
    }
    const expandedKey = `running-tasks-card-expanded-${cascadeId}`;
    const storedExpanded = localStorage.getItem(expandedKey);
    const defaultExpanded = !window.CompactModePrefs?.shouldCollapseTasks?.();
    this.isExpanded = (storedExpanded !== null) ? (storedExpanded === "true") : defaultExpanded;
    this.render();
  },

  syncCompactDefault() {
    this.isExpanded = !window.CompactModePrefs?.shouldCollapseTasks?.();
    if (this.cascadeId) {
      localStorage.removeItem(`running-tasks-card-expanded-${this.cascadeId}`);
    }
    this.render();
  },

  syncFromServer(serverTasks) {
    if (!Array.isArray(serverTasks)) {
      this.tasks = [];
    } else {
      this.tasks = serverTasks;
    }
    this.render();
  },

  toggleExpand() {
    this.isExpanded = !this.isExpanded;
    if (activeCascadeId) {
      localStorage.setItem(`running-tasks-card-expanded-${activeCascadeId}`, String(this.isExpanded));
    }
    this.render();
  },

  async stopTask(stepIndex, taskId, skipConfirm = false) {
    if (!activeCascadeId) return;
    if (!skipConfirm && !confirm(typeof t === "function" ? t("确定要终止此后台任务吗？") : "确定要终止此后台任务吗？")) return;

    try {
      const resp = await fetch("/gateway/cascade/task/stop", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          cascadeId: activeCascadeId,
          stepIndex: stepIndex,
          taskId: taskId || `task-${stepIndex}`
        })
      });
      if (!resp.ok) {
        const data = await resp.json().catch(() => ({}));
        throw new Error(data.error || `HTTP ${resp.status}`);
      }
      // Optimistically remove from local tasks list
      this.tasks = this.tasks.filter(t => t.stepIndex !== stepIndex && t.id !== taskId);
      this.render();
    } catch (err) {
      alert(typeof t === "function" ? t("终止任务失败: " + err.message) : "终止任务失败: " + err.message);
    }
  },

  render() {
    const cardEl = document.getElementById("running-tasks-card");
    const countEl = document.getElementById("tasks-badge-count");
    const titleEl = document.getElementById("tasks-header-title");
    const wrapperEl = document.getElementById("tasks-content-wrapper");
    const arrowEl = cardEl?.querySelector(".expand-arrow");
    const listEl = document.getElementById("tasks-items-list");
    if (!cardEl || !countEl || !wrapperEl || !listEl) return;

    if (this.tasks.length === 0) {
      cardEl.classList.add("hidden");
      return;
    }

    cardEl.classList.remove("hidden");
    countEl.textContent = this.tasks.length;
    if (titleEl) {
      titleEl.textContent = `${this.tasks.length} 个任务正在执行`;
    }

    if (this.isExpanded) {
      wrapperEl.classList.remove("collapsed");
      arrowEl?.classList.remove("collapsed");
    } else {
      wrapperEl.classList.add("collapsed");
      arrowEl?.classList.add("collapsed");
    }

    const quickStopEl = document.getElementById("tasks-header-quick-stop");
    if (quickStopEl) {
      if (!this.isExpanded && this.tasks.length === 1) {
        const t = this.tasks[0];
        const tIdEsc = escapeHtml(t.id || "");
        quickStopEl.innerHTML = `
          <button type="button" class="task-stop-btn" data-action="stop-task" data-step="${Number(t.stepIndex) || 0}" data-id="${tIdEsc}" title="终止任务" aria-label="终止任务">
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 -960 960 960" fill="currentColor">
              <path d="M330-330H630V-630H330v300ZM480.07-100q-78.84,0-148.2-29.92T211.18-211.13T129.93-331.76T100-479.93t29.92-148.2t81.21-120.68t120.63-81.25T479.93-860t148.2,29.92t120.68,81.21t81.25,120.63T860-480.07t-29.92,148.2T748.87-211.18T628.24-129.93T480.07-100ZM480-160q134,0 227-93t93-227T707-707T480-800T253-707T160-480t93,227t227,93Zm0-320Z"></path>
            </svg>
          </button>
        `;
      } else {
        quickStopEl.innerHTML = "";
      }
    }

    listEl.innerHTML = this.tasks.map(task => {
      const desc = escapeHtml(task.toolSummary || task.toolAction || formatToolName(task.toolName) || "后台任务");
      const rawCmd = task.commandLine || "运行中...";
      const cmd = escapeHtml(rawCmd);
      const idEsc = escapeHtml(task.id || "");
      return `
        <div class="task-item-row" data-id="${idEsc}" data-step="${task.stepIndex}">
          <div class="task-item-body">
            <span class="task-item-desc">${desc}</span>
            <span class="task-item-cmd" data-cmd="${cmd}" title="点击复制命令: ${cmd}" onclick="copyTaskCommandEl(this)">${cmd}</span>
          </div>
          <div class="p3-btn-group">
            <button type="button" class="p3-mini-btn" onclick="window.Phase3?.sendTaskToBackground?.(${task.stepIndex})" title="转入后台">转后台</button>
            <button type="button" class="p3-mini-btn" onclick="window.Phase3?.openToolsBoard?.('tasks')" title="交互终端">终端</button>
            <button class="task-stop-btn" data-action="stop-task" data-step="${task.stepIndex}" data-id="${idEsc}" title="终止任务" aria-label="终止任务">
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 -960 960 960" fill="currentColor">
              <path d="M330-330H630V-630H330v300ZM480.07-100q-78.84,0-148.2-29.92T211.18-211.13T129.93-331.76T100-479.93t29.92-148.2t81.21-120.68t120.63-81.25T479.93-860t148.2,29.92t120.68,81.21t81.25,120.63T860-480.07t-29.92,148.2T748.87-211.18T628.24-129.93T480.07-100ZM480-160q134,0 227-93t93-227T707-707T480-800T253-707T160-480t93,227t227,93Zm0-320Z"></path>
            </svg>
          </button>
        </div>
      `;
    }).join("");
  }
};

function copyTaskCommandEl(el) {
  if (!el) return;
  const cmd = el.getAttribute("data-cmd") || el.textContent || "";
  if (!cmd) return;
  navigator.clipboard?.writeText(cmd).then(() => {
    const prevText = el.textContent;
    el.classList.add("copied");
    el.textContent = "✓ 已复制命令";
    setTimeout(() => {
      el.classList.remove("copied");
      el.textContent = prevText;
    }, 1300);
  }).catch(() => {});
}

// --- Local Message Queue Manager (Desktop Parity) ---
const LocalQueueManager = {
  queue: [],
  isExpanded: true,
  deletedTombstones: [],

  init(cascadeId) {
    this.deletedTombstones = [];
    if (!cascadeId) {
      this.queue = [];
      this.render();
      return;
    }
    const expandedKey = `queued-messages-card-expanded-${cascadeId}`;
    const storedExpanded = localStorage.getItem(expandedKey);
    this.isExpanded = (storedExpanded !== null) ? (storedExpanded === "true") : true;

    const storedQueue = localStorage.getItem(`agy_queue_${cascadeId}`);
    if (storedQueue) {
      try {
        this.queue = JSON.parse(storedQueue);
      } catch (e) {
        this.queue = [];
      }
    } else {
      this.queue = [];
    }
    this.render();
  },

  save() {
    if (!activeCascadeId) return;
    localStorage.setItem(`agy_queue_${activeCascadeId}`, JSON.stringify(this.queue));
    this.render();
  },

  normalizeForComparison(txt) {
    if (!txt) return '';
    return String(txt)
      .toLowerCase()
      .replace(/[\s\u200B\uFEFF\u3000]/g, '');
  },

  isQueuedItemInMessages(item, userItems) {
    const normText = this.normalizeForComparison(item.text);
    const hasAttachments = (item.media && item.media.length > 0) || (item.imageUrls && item.imageUrls.length > 0);
    
    for (let i = userItems.length - 1; i >= 0; i--) {
      const u = userItems[i];
      const normMsg = this.normalizeForComparison(u.text);
      const msgHasAttachments = u.hasAttachments;
      
      if (normText) {
        if (normText === normMsg) return true;
        if (normText.length >= 6 && normMsg.length >= 6 && (normText.includes(normMsg) || normMsg.includes(normText))) {
          return true;
        }
      } else if (hasAttachments && msgHasAttachments) {
        return true;
      }
    }
    return false;
  },

  syncFromServer(serverQueue, currentStepsOrMessages = []) {
    const userItems = [];
    if (Array.isArray(currentStepsOrMessages)) {
      for (const item of currentStepsOrMessages) {
        if (item && item.type === 'CORTEX_STEP_TYPE_USER_INPUT' && item.userInput) {
          const t = (item.userInput.userResponse || item.userInput.response || '').trim();
          const hasMedia = (item.userInput.media && item.userInput.media.length > 0) || (item.userInput.images && item.userInput.images.length > 0);
          userItems.push({ text: t, hasAttachments: hasMedia });
        } else if (item && (item.sender === 'user' || item.role === 'user' || item.type === 'user')) {
          const t = (item.content || item.text || '').trim();
          const hasMedia = (item.media && item.media.length > 0) || (item.imageUrls && item.imageUrls.length > 0);
          userItems.push({ text: t, hasAttachments: hasMedia });
        }
      }
    }

    const now = Date.now();
    this.deletedTombstones = (this.deletedTombstones || []).filter(t => (now - t.deletedAt) < 10000);
    const tombstoneIds = new Set(this.deletedTombstones.filter(t => t.id).map(t => t.id));
    const tombstoneTexts = new Set(this.deletedTombstones.map(t => this.normalizeForComparison(t.text)));

    const isUserMsg = (txt, item) => {
      const t = (txt || '').trim();
      const hasMedia = item && ((item.media && item.media.length > 0) || (item.imageUrls && item.imageUrls.length > 0));
      if (!t && !hasMedia) return false;
      if (t.startsWith('Task id "') || t.startsWith('Task "') || t.includes('was canceled with result:') || t.includes('completed with result:') || t.includes('Tool execution was canceled')) {
        return false;
      }
      return true;
    };

    if (Array.isArray(serverQueue)) {
      const pendingOpt = this.queue.filter(it => {
        if (!it.id || !it.id.startsWith('queue-')) return false;
        if (now - new Date(it.createdAt).getTime() >= 15000) return false;
        const norm = this.normalizeForComparison(it.text);
        if (tombstoneIds.has(it.id) || (norm && tombstoneTexts.has(norm))) return false;
        if (this.isQueuedItemInMessages(it, userItems)) {
          this.deletedTombstones.push({ id: it.id, text: it.text, deletedAt: now });
          return false;
        }
        if (serverQueue.some(s => this.normalizeForComparison(s.text) === norm)) return false;
        return true;
      });

      const baseQueue = serverQueue
        .filter(item => {
          const norm = this.normalizeForComparison(item.text);
          if (!isUserMsg(item.text, item)) return false;
          if (tombstoneIds.has(item.id) || (norm && tombstoneTexts.has(norm))) return false;
          if (this.isQueuedItemInMessages(item, userItems)) {
            this.deletedTombstones.push({ id: item.id, text: item.text, deletedAt: now });
            return false;
          }
          return true;
        })
        .map(item => ({
          id: item.id || `server-${Date.now()}`,
          text: item.text,
          media: item.media,
          imageUrls: item.imageUrls,
          createdAt: item.createdAt || new Date().toISOString()
        }));

      this.queue = [...baseQueue, ...pendingOpt];
    } else {
      this.queue = this.queue.filter(item => {
        const norm = this.normalizeForComparison(item.text);
        if (!isUserMsg(item.text, item)) return false;
        if (tombstoneIds.has(item.id) || (norm && tombstoneTexts.has(norm))) return false;
        if (this.isQueuedItemInMessages(item, userItems)) {
          this.deletedTombstones.push({ id: item.id, text: item.text, deletedAt: now });
          return false;
        }
        return true;
      });
    }
    this.save();
  },

  enqueue(text, clientMessageId = null) {
    const t = (text || '').trim();
    if (!t) return null;
    if (this.deletedTombstones) {
      this.deletedTombstones = this.deletedTombstones.filter(it => (it.text || '').trim() !== t);
    }
    const item = {
      id: `queue-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`,
      text: t,
      clientMessageId: clientMessageId || undefined,
      createdAt: new Date().toISOString()
    };
    this.queue.push(item);
    this.save();
    return item.id;
  },

  discardOptimistic(id, cascadeId = null) {
    if (!id) return;
    const target = cascadeId || activeCascadeId;
    if (!target || target === activeCascadeId) {
      this.queue = this.queue.filter(it => it.id !== id);
      this.save();
      return;
    }
    try {
      const key = `agy_queue_${target}`;
      const raw = localStorage.getItem(key);
      if (raw) {
        const list = JSON.parse(raw).filter(it => it.id !== id);
        localStorage.setItem(key, JSON.stringify(list));
      }
    } catch (_) {}
  },

  remove(id) {
    const item = this.queue.find(it => it.id === id);
    if (item) {
      this.deletedTombstones = this.deletedTombstones || [];
      this.deletedTombstones.push({
        id: item.id,
        text: (item.text || '').trim(),
        deletedAt: Date.now()
      });
    }
    this.queue = this.queue.filter(it => it.id !== id);
    this.save();
    if (activeCascadeId) {
      if (id && !id.startsWith("queue-")) {
        rpc("DeleteAgentMessage", { messageId: id, recipient: activeCascadeId }).catch(() => {});
      } else if (item && item.text) {
        setTimeout(async () => {
          try {
            const info = await rpc("GetCascadeTrajectory", { cascadeId: activeCascadeId });
            const serverQueue = info?.queuedMessages || [];
            const match = serverQueue.find(s => (s.text || "").trim() === item.text.trim());
            if (match && match.id && !match.id.startsWith("queue-")) {
              if (this.deletedTombstones) {
                this.deletedTombstones.push({
                  id: match.id,
                  text: (item.text || '').trim(),
                  deletedAt: Date.now()
                });
              }
              rpc("DeleteAgentMessage", { messageId: match.id, recipient: activeCascadeId }).catch(() => {});
            }
          } catch (_) {}
        }, 350);
      }
    }
  },

  async sendNow(id) {
    const item = this.queue.find(it => it.id === id);
    if (!item || !activeCascadeId) return;

    // Optimistic removal from queue UI
    const originalQueue = [...this.queue];
    this.queue = this.queue.filter(it => it.id !== id);
    this.save();

    // Optimistically render user message in chat stream
    const streamEl = document.getElementById("messages-stream");
    const tempId = `temp-user-${Date.now()}`;
    if (streamEl) {
      const textHtml = item.text ? `<div>${escapeHtml(item.text)}</div>` : "";
      streamEl.insertAdjacentHTML("beforeend", `
        <div id="${tempId}" class="message-row user">
          <div class="bubble">${textHtml}</div>
        </div>
      `);
      userIsNearBottom = true;
      streamEl.scrollTop = streamEl.scrollHeight;
    }

    try {
      if (currentTrajectories[activeCascadeId]) {
        currentTrajectories[activeCascadeId].status = "CASCADE_RUN_STATUS_RUNNING";
        currentTrajectories[activeCascadeId].needsInput = false;
      }
      updateChatControls(true, null, false);
      const clientMsgId = "web-" + Date.now() + "-" + Math.random().toString(36).slice(2);
      await rpc("SendUserCascadeMessage", {
        cascadeId: activeCascadeId,
        model: activeModel,
        items: [{ text: item.text }],
        deliveryStrategy: 1 // NEXT_INVOCATION
      }, { "X-Client-Message-Id": clientMsgId });

      // After successful send, delete from server queue
      if (id && !id.startsWith("queue-")) {
        rpc("DeleteAgentMessage", { messageId: id, recipient: activeCascadeId }).catch(() => {});
      } else {
        setTimeout(async () => {
          try {
            const info = await rpc("GetCascadeTrajectory", { cascadeId: activeCascadeId });
            const serverQueue = info?.queuedMessages || [];
            const match = serverQueue.find(s => (s.text || "").trim() === item.text.trim());
            if (match && match.id && !match.id.startsWith("queue-")) {
              rpc("DeleteAgentMessage", { messageId: match.id, recipient: activeCascadeId }).catch(() => {});
            }
          } catch (_) {}
        }, 350);
      }

      if (!activeWs || activeWs.readyState !== WebSocket.OPEN) {
        connectStreamWs(activeCascadeId);
      }
    } catch (err) {
      const tempEl = document.getElementById(tempId);
      if (tempEl) tempEl.remove();
      this.queue = originalQueue;
      this.save();
      alert(typeof t === "function" ? t("发送失败: " + err.message) : "发送失败: " + err.message);
    }
  },

  edit(id) {
    const item = this.queue.find(it => it.id === id);
    if (!item) return;
    this.remove(id);

    const inputEl = document.getElementById("chat-input");
    if (inputEl) {
      inputEl.value = item.text;
      inputEl.style.height = "auto";
      inputEl.style.height = Math.min(inputEl.scrollHeight, 120) + "px";
      inputEl.focus();
      const isRunning = isCascadeRunning(activeCascadeId);
      updateChatControls(isRunning, null, false);
    }
  },

  toggleExpand() {
    this.isExpanded = !this.isExpanded;
    if (activeCascadeId) {
      localStorage.setItem(`queued-messages-card-expanded-${activeCascadeId}`, String(this.isExpanded));
    }
    this.render();
  },

  onAgentCompleted() {
    // Upstream LanguageServer automatically triggers WHEN_IDLE queued messages.
    // Client-side auto-dispatch is disabled to prevent duplicate triggers.
  },

  render() {
    const cardEl = document.getElementById("queued-messages-card");
    const countEl = document.getElementById("queued-badge-count");
    const wrapperEl = document.getElementById("queued-content-wrapper");
    const arrowEl = cardEl?.querySelector(".expand-arrow");
    const listEl = document.getElementById("queued-items-list");
    if (!cardEl || !countEl || !wrapperEl || !listEl) return;

    if (this.queue.length === 0) {
      cardEl.classList.add("hidden");
      return;
    }

    cardEl.classList.remove("hidden");
    countEl.textContent = this.queue.length;

    const subEl = cardEl.querySelector(".queued-subtitle");
    if (subEl) {
      const blockers = [];
      if (typeof SubagentManager !== "undefined" && Array.isArray(SubagentManager.items)) {
        for (const sa of SubagentManager.items) {
          if (sa && sa.status === "running") {
            const name = (sa.role || sa.typeName || "").trim();
            if (name) blockers.push(name);
          }
        }
      }
      const rTasks = (typeof RunningTasksManager !== "undefined" && Array.isArray(RunningTasksManager.tasks))
        ? RunningTasksManager.tasks : [];
      const nonDaemon = rTasks.filter((t) => !t.isDaemon);
      for (const t of (nonDaemon.length > 0 ? nonDaemon : rTasks)) {
        const raw = (t.toolSummary || t.commandLine || t.toolName || "").trim();
        if (raw) blockers.push(raw.length > 18 ? raw.slice(0, 18) + "…" : raw);
      }
      if (blockers.length > 0) {
        const head = blockers.slice(0, 2).join("、");
        subEl.textContent = blockers.length > 2
          ? `等待 ${head} 等${blockers.length}项完成后出列`
          : `等待 ${head} 完成后出列`;
        subEl.style.color = "#f59e0b";
      } else {
        subEl.textContent = "当前任务完成后自动发送";
        subEl.style.color = "";
      }
    }

    if (this.isExpanded) {
      wrapperEl.classList.remove("collapsed");
      arrowEl?.classList.remove("collapsed");
    } else {
      wrapperEl.classList.add("collapsed");
      arrowEl?.classList.add("collapsed");
    }

    listEl.innerHTML = this.queue.map(item => {
      let thumbHtml = "";
      if (item.media && item.media.length > 0) {
        const raw = item.media[0];
        const src = raw.startsWith("data:") ? raw : `data:image/jpeg;base64,${raw}`;
        thumbHtml = `<img class="queued-item-thumb" src="${escapeHtml(src)}" alt="attachment" />`;
      } else if (item.imageUrls && item.imageUrls.length > 0) {
        const thumbSrc = typeof resolveThumbnailRawUrl === "function" ? resolveThumbnailRawUrl(item.imageUrls[0]) : item.imageUrls[0];
        thumbHtml = `<img class="queued-item-thumb" src="${escapeHtml(thumbSrc)}" alt="attachment" />`;
      }
      const safeId = escapeHtml(String(item.id ?? ""));
      return `
      <div class="queued-item-row" data-id="${safeId}">
        ${thumbHtml}
        <span class="queued-item-text">${escapeHtml(item.text || (thumbHtml ? "图片" : ""))}</span>
        <div class="queued-actions" data-testid="queued-decorators">
          <button class="queued-icon-btn btn-send-now" data-action="queue-send" data-id="${safeId}" title="立即发送" aria-label="立即发送">
            <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 -960 960 960" fill="currentColor">
              <path d="M665.08-450H180v-60H665.08L437.23-737.85L480-780L780-480L480-180l-42.77-42.15L665.08-450Z"></path>
            </svg>
          </button>
          <button class="queued-icon-btn btn-edit" data-action="queue-edit" data-id="${safeId}" title="编辑" aria-label="编辑">
            <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 -960 960 960" fill="currentColor">
              <path d="M200-200h50.46L659.92-609.46l-50.46-50.46L200-250.46V-200Zm-60,60V-275.38L667.62-802.77q9.07-8.24 20.04-12.74T710.65-820t23.31,4.27t19.97,13.58l48.85,49.46q9.31,8.69 13.27,20T820-710.07q0,12.07-4.12,23.03T802.77-667L275.38-140H140ZM760.38-710.15l-50.23-50.23l50.23,50.23Zm-126.13,75.9l-24.79-25.67l50.46,50.46l-25.67-24.79Z"></path>
            </svg>
          </button>
          <button class="queued-icon-btn btn-delete" data-action="queue-remove" data-id="${safeId}" title="删除" aria-label="删除">
            <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 -960 960 960" fill="currentColor">
              <path d="M292.31-140q-29.92,0-51.11-21.19T220-212.31V-720H180v-60H360v-35.38H600V-780H780v60H740v507.69Q740-182 719-161t-51.31,21H292.31ZM680-720H280v507.69q0,5.39 3.46,8.85t8.85,3.46H667.69q4.62,0 8.46-3.85t3.85-8.46V-720ZM376.16-280h60V-640h-60v360Zm147.69,0h60V-640h-60v360ZM280-720v507.69q0,5.39 0,8.85t0,3.46q0,0 0-3.46t0-8.85V-720Z"></path>
            </svg>
          </button>
        </div>
      </div>
    `;}).join("");
  }
};

// 语音连接失败后「一键排查」发送给智能体的预设提示词（按当前界面语言选择 zh / en / ja，三端保持一致）
const VOICE_TROUBLESHOOT_SHOWN_KEY = "ag_voice_troubleshoot_bubble_shown";

function getVoiceTroubleshootPrompt() {
  const lang = (typeof window !== "undefined" && window.I18n && typeof window.I18n.getLanguage === "function")
    ? window.I18n.getLanguage()
    : "en";
  if (lang === "zh-CN") {
    return [
      "【任务】排查「本机无法使用语音（Gemini Live）功能」的问题。",
      "",
      "【现象】",
      "- 实时语音通话无法建立或意外中断，界面提示无法连接（如 “Couldn't connect / Try again”）",
      "",
      "【请按顺序排查】",
      "1. 登录与授权：",
      "   - 检查当前登录态的 OAuth Scope 是否完整；旧登录态可能缺少语音所需权限",
      "   - 如需重新授权，先退出 Antigravity，将 jetski-standalone-oauth-token 文件重命名或移走，再重新启动并登录以拉取完整 Scope",
      "2. 网络、防火墙与代理长连接：",
      "   - 语音依赖对 *.googleapis.com 的双向流式长连接（HTTP/2 / gRPC bidi stream）",
      "   - 检查当前网络、VPN 或防火墙是否拦截了该长连接；如本机使用代理工具（如 Clash / Sing-box 等），确认已开启 TUN 模式或支持 HTTP/2 与 gRPC 转发",
      "   - 检查日志中是否出现 context canceled while waiting for connections 或连接超时特征",
      "3. 系统麦克风：",
      "   - 确认系统麦克风权限已授予桌面客户端，且默认输入设备正常、未被其他程序占用",
      "4. 网关链路：",
      "   - 检查本地网关语音 WebSocket（/voice/ws）链路与日志（mgy 会话输出）",
      "",
      "【输出要求】",
      "- 先给出根因判断与证据（日志或命令输出），再给修复步骤",
      "- 能自动修复的直接修复；涉及改配置、重启服务或重新登录的，先说明再执行",
      "- 修复后验证语音是否恢复"
    ].join("\n");
  }
  if (lang === "ja") {
    return [
      "【タスク】このPCでリアルタイム音声（Gemini Live）に接続できない・途中で切断される原因を調査してください。",
      "",
      "【現象】",
      "- 音声通話の接続に失敗する、または予期せず切断される（「Couldn't connect / Try again」など）。",
      "",
      "【確認手順（上から順に）】",
      "1. ログイン状態と OAuth 権限：",
      "   - 現在の OAuth トークンに音声機能に必要なスコープが揃っているか確認してください（古いセッションでは権限が不足している場合があります）。",
      "   - 再認可が必要な場合は、Antigravity を完全に終了し、`jetski-standalone-oauth-token` をリネームまたは移動してから再起動・再ログインして全権限を取得してください。",
      "2. ネットワーク・ファイアウォール・VPN（HTTP/2・gRPC 接続）：",
      "   - 音声機能は `*.googleapis.com` への双方向ストリーミング通信（HTTP/2 / gRPC）を使用します。",
      "   - ネットワーク環境、VPN、ファイアウォール、またはプロキシが HTTP/2・gRPC の常時接続を遮断・切断していないか確認してください。",
      "   - ログに `context canceled while waiting for connections` などのタイムアウト・切断エラーが出ていないか確認してください。",
      "3. PC のマイク設定：",
      "   - OS のマイクアクセス権限がデスクトップアプリに許可されていること、および既定の入力デバイスが正常で他アプリに占有されていないことを確認してください。",
      "4. ゲートウェイ接続：",
      "   - ローカルゲートウェイの `/voice/ws` WebSocket 経路および `mgy` ログを確認してください。",
      "",
      "【出力要件】",
      "- まず原因の特定結果と根拠（ログやコマンド出力）を示し、その後に修復手順を提示してください。",
      "- 安全に自動修復できる項目はそのまま対応し、設定変更・サービス再起動・再ログインが必要な場合は事前に説明してから実行してください。",
      "- 修復後、音声機能が正常に復旧したか確認してください。"
    ].join("\n");
  }
  return [
    "[Task] Diagnose why real-time voice (Gemini Live) cannot connect or was unexpectedly interrupted on this computer.",
    "",
    "[Symptoms]",
    "- Voice calls fail to connect or drop unexpectedly (e.g. \"Couldn't connect / Try again\").",
    "",
    "[Troubleshooting Steps (in order)]",
    "1. Sign-in & OAuth Scope:",
    "   - Check whether the current OAuth token includes all required voice scopes; older sessions may be missing voice permissions.",
    "   - If re-authorization is needed, quit Antigravity completely, rename or move the `jetski-standalone-oauth-token` file, then relaunch and sign in again to refresh scopes.",
    "2. Network, Firewall, VPN & gRPC Streaming:",
    "   - Real-time voice requires persistent bidirectional HTTP/2 / gRPC streaming to `*.googleapis.com`.",
    "   - Check whether the network, VPN, corporate firewall, or proxy is dropping or blocking HTTP/2 / gRPC connections.",
    "   - Inspect logs for timeout or stream errors such as `context canceled while waiting for connections`.",
    "3. System Microphone:",
    "   - Verify that microphone access is granted to the desktop client and that the default input device is active and not exclusively held by another app.",
    "4. Local Gateway Link:",
    "   - Check the local gateway `/voice/ws` WebSocket relay and `mgy` logs for upstream errors.",
    "",
    "[Output Requirements]",
    "- Report the root cause with concrete evidence (logs or command output) first, followed by fix steps.",
    "- Apply safe fixes automatically; explain before changing configs, restarting services, or requiring re-login.",
    "- Verify whether voice connectivity is restored after the fix."
  ].join("\n");
}

// --- Full-Duplex Real-time Voice Call Manager (jetski-voice-proto) ---
const WebVoiceCallManager = {
  _troubleshootBubbleShown: false,
  isInCall: false,
  callState: "idle", // idle | connecting | listening | reconnecting

  hasShownTroubleshootBubble() {
    if (this._troubleshootBubbleShown) return true;
    try {
      if (localStorage.getItem(VOICE_TROUBLESHOOT_SHOWN_KEY) === "true") {
        this._troubleshootBubbleShown = true;
        return true;
      }
    } catch (_) {}
    return false;
  },

  markTroubleshootBubbleShown() {
    this._troubleshootBubbleShown = true;
    try {
      localStorage.setItem(VOICE_TROUBLESHOOT_SHOWN_KEY, "true");
    } catch (_) {}
  },

  isVoiceModeEnabled() {
    try {
      return localStorage.getItem("ag_enable_voice_mode") === "true";
    } catch (_) {
      return false;
    }
  },

  setVoiceModeEnabled(enabled) {
    try {
      localStorage.setItem("ag_enable_voice_mode", enabled ? "true" : "false");
    } catch (_) {}
    if (!enabled && this.isInCall) {
      this.endCall();
    } else {
      updateChatControls(isCascadeRunning(activeCascadeId), null, false);
    }
  },
  isMicMuted: false,
  isSpeakerMuted: false,
  playbackSpeed: 1.0,
  supportedSpeeds: [0.75, 1.0, 1.25, 1.5, 1.75, 2.0],
  isAgentSpeaking: false,
  soundLevels: new Array(16).fill(0.14),
  ws: null,
  audioCtx: null,
  mediaStream: null,
  scriptNode: null,
  gainNode: null,
  nextPlayTime: 0,
  activeSources: [],
  durationTimer: null,
  startedAt: 0,
  subtitleTimer: null,
  agentSpeakingTimer: null,

  initUI() {
    const barsEl = document.getElementById("voice-waveform-bars");
    if (barsEl && !barsEl.children.length) {
      barsEl.innerHTML = this.soundLevels.map(() => `<span class="voice-wave-bar"></span>`).join("");
    }
    if (this._uiInitialized) return;
    this._uiInitialized = true;
    document.getElementById("btn-vtb-retry")?.addEventListener("click", () => this.hideTroubleshootBubble());
    document.getElementById("btn-vtb-troubleshoot")?.addEventListener("click", () => this.sendTroubleshootPrompt());
    document.getElementById("btn-voice-speed")?.addEventListener("click", () => this.cycleSpeed());
    document.getElementById("btn-voice-mic")?.addEventListener("click", () => this.toggleMicMute());
    document.getElementById("btn-voice-speaker")?.addEventListener("click", () => this.toggleSpeakerMute());
    document.getElementById("btn-voice-stop-task")?.addEventListener("click", () => {
      if (typeof cancelCurrentTask === "function") cancelCurrentTask();
    });
  },

  async startCall() {
    if (this.isInCall) {
      this.endCall();
      return;
    }
    this.initUI();

    try {
      this.mediaStream = await navigator.mediaDevices.getUserMedia({
        audio: {
          channelCount: 1,
          sampleRate: 16000,
          echoCancellation: true,
          noiseSuppression: true,
          autoGainControl: false
        }
      });
    } catch (err) {
      showToast("需要麦克风权限才能开启实时语音对话");
      return;
    }

    // Ensure cascadeId exists if currently in #draft mode
    let targetCascadeId = activeCascadeId;
    if (!targetCascadeId) {
      try {
        const draft = activeDraftSession || { isPure: true, uri: "", rawId: "outside-of-project" };
        const res = await fetch("/gateway/cascade/new", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            workspaceUri: draft.isPure ? "" : (draft.uri || ""),
            prompt: "",
            model: activeModel || "",
            projectId: draft.isPure ? "outside-of-project" : (draft.rawId || "")
          })
        });
        const data = await res.json();
        if (data && data.cascadeId) {
          targetCascadeId = data.cascadeId;
          activeCascadeId = targetCascadeId;
          window.history.replaceState({}, document.title, `#c=${targetCascadeId}`);
          connectStreamWs(targetCascadeId);
        }
      } catch (e) {
        showToast("无法创建语音会话");
        this.cleanupMedia();
        return;
      }
    }

    this.isInCall = true;
    this.callState = "connecting";
    this.isMicMuted = false;
    this.isSpeakerMuted = false;
    this.isAgentSpeaking = false;
    this.startedAt = Date.now();
    this.setupAudioContext();
    this.renderCard();
    updateChatControls(isCascadeRunning(activeCascadeId), null, false);

    this.durationTimer = setInterval(() => this.updateTimerUI(), 500);

    const proto = location.protocol === "https:" ? "wss:" : "ws:";
    let wsUrl = `${proto}//${location.host}/voice/ws?conversation_id=${encodeURIComponent(targetCascadeId || "")}&plan_model=${encodeURIComponent(activeModel || "")}&sample_rate=16000`;
    try {
      const resp = await originalFetch("/api/v1/auth/ws-ticket", { method: "POST" });
      if (resp.ok) {
        const data = await resp.json();
        if (data && data.ticket) {
          wsUrl += `&ticket=${encodeURIComponent(data.ticket)}`;
        }
      }
    } catch (_) {}

    if (!this.isInCall) return;

    const ws = new WebSocket(wsUrl, ["jetski-voice-proto"]);
    ws.binaryType = "arraybuffer";
    this.ws = ws;

    ws.onopen = () => {
      if (this.ws !== ws) return;
      this.callState = "listening";
      if (this.playbackSpeed !== 1.0) {
        this.sendSettingsUpdate(null, this.playbackSpeed);
      }
      this.renderCard();
    };

    ws.onmessage = (ev) => {
      if (this.ws !== ws || !(ev.data instanceof ArrayBuffer)) return;
      this.handleServerFrame(new Uint8Array(ev.data));
    };

    ws.onclose = () => {
      if (this.ws === ws && this.isInCall) {
        this.endCall();
        this.showTroubleshootBubble();
      }
    };

    ws.onerror = () => {
      if (this.ws === ws && this.isInCall) {
        showToast("实时语音连接断开");
        this.endCall();
        this.showTroubleshootBubble();
      }
    };
  },

  // 连接明确失败（已自动挂断）后弹出排查气泡；安装/首次记录后永久只弹一次
  showTroubleshootBubble() {
    if (this.hasShownTroubleshootBubble()) return;
    this.markTroubleshootBubbleShown();
    document.getElementById("voice-troubleshoot-bubble")?.classList.remove("hidden");
  },

  hideTroubleshootBubble() {
    document.getElementById("voice-troubleshoot-bubble")?.classList.add("hidden");
  },

  async sendTroubleshootPrompt() {
    this.hideTroubleshootBubble();
    const inputEl = document.getElementById("chat-input");
    if (!inputEl || typeof sendMessage !== "function") return;
    const prevDraft = inputEl.value;
    inputEl.value = getVoiceTroubleshootPrompt();
    await sendMessage();
    if (prevDraft && !inputEl.value) inputEl.value = prevDraft;
  },

  endCall() {
    this.isInCall = false;
    this.callState = "idle";
    this.isAgentSpeaking = false;
    if (this.durationTimer) {
      clearInterval(this.durationTimer);
      this.durationTimer = null;
    }
    if (this.subtitleTimer) {
      clearTimeout(this.subtitleTimer);
      this.subtitleTimer = null;
    }
    if (this.agentSpeakingTimer) {
      clearTimeout(this.agentSpeakingTimer);
      this.agentSpeakingTimer = null;
    }
    if (this.ws) {
      try { this.ws.close(1000, "Hang up"); } catch (_) {}
      this.ws = null;
    }
    this.cleanupMedia();
    const card = document.getElementById("voice-call-card");
    if (card) card.classList.add("hidden");
    const subRow = document.getElementById("voice-live-subtitle-row");
    if (subRow) subRow.classList.add("hidden");
    updateChatControls(isCascadeRunning(activeCascadeId), null, false);
  },

  cleanupMedia() {
    this.clearPlayback();
    if (this.scriptNode) {
      try { this.scriptNode.disconnect(); } catch (_) {}
      this.scriptNode = null;
    }
    if (this.mediaStream) {
      this.mediaStream.getTracks().forEach(t => t.stop());
      this.mediaStream = null;
    }
    if (this.audioCtx) {
      try { this.audioCtx.close(); } catch (_) {}
      this.audioCtx = null;
    }
    this.gainNode = null;
  },

  setupAudioContext() {
    const AudioContextClass = window.AudioContext || window.webkitAudioContext;
    const ctx = new AudioContextClass();
    this.audioCtx = ctx;
    this.gainNode = ctx.createGain();
    this.gainNode.gain.value = this.isSpeakerMuted ? 0 : 1;
    this.gainNode.connect(ctx.destination);
    this.nextPlayTime = 0;

    const source = ctx.createMediaStreamSource(this.mediaStream);
    const proc = ctx.createScriptProcessor(2048, 1, 1);
    this.scriptNode = proc;
    let sampleAcc = [];

    proc.onaudioprocess = (e) => {
      if (!this.isInCall) return;
      const input = e.inputBuffer.getChannelData(0);
      const inRate = ctx.sampleRate || 48000;
      const ratio = inRate / 16000;
      let sumSq = 0;
      for (let i = 0; i < input.length; i += ratio) {
        const idx = Math.min(input.length - 1, Math.floor(i));
        const s = Math.max(-1, Math.min(1, input[idx]));
        sumSq += s * s;
        sampleAcc.push(s < 0 ? Math.round(s * 32768) : Math.round(s * 32767));
      }
      if (!this.isAgentSpeaking) {
        const rms = Math.sqrt(sumSq / Math.max(1, input.length / ratio));
        this.pushWaveLevel(this.isMicMuted ? 0.10 : Math.min(1.0, Math.max(0.12, rms * 5.2)));
      }
      while (sampleAcc.length >= 640) {
        const chunkSamples = sampleAcc.splice(0, 640);
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
          const pcmBytes = new Uint8Array(1280);
          if (!this.isMicMuted) {
            const view = new DataView(pcmBytes.buffer);
            for (let j = 0; j < 640; j++) {
              view.setInt16(j * 2, chunkSamples[j], true);
            }
          }
          const frame = new Uint8Array(3 + 1280);
          frame[0] = 0x12; // field 2, wire type 2
          frame[1] = 0x80; // varint 1280 low
          frame[2] = 0x0a; // varint 1280 high
          frame.set(pcmBytes, 3);
          this.ws.send(frame.buffer);
        }
      }
    };

    source.connect(proc);
    proc.connect(ctx.destination);
  },

  enqueuePcm24k(pcmBytes) {
    if (!this.audioCtx || !this.gainNode) return;
    const sampleCount = Math.floor(pcmBytes.byteLength / 2);
    if (sampleCount <= 0) return;

    const audioBuf = this.audioCtx.createBuffer(1, sampleCount, 24000);
    const ch = audioBuf.getChannelData(0);
    const view = new DataView(pcmBytes.buffer, pcmBytes.byteOffset, pcmBytes.byteLength);
    let sumSq = 0;
    for (let i = 0; i < sampleCount; i++) {
      const f = view.getInt16(i * 2, true) / 32768;
      ch[i] = f;
      sumSq += f * f;
    }

    const rms = Math.sqrt(sumSq / sampleCount);
    this.isAgentSpeaking = true;
    this.pushWaveLevel(Math.min(1.0, Math.max(0.16, rms * 4.5)));
    this.renderCard();

    const src = this.audioCtx.createBufferSource();
    src.buffer = audioBuf;
    src.connect(this.gainNode);
    const now = this.audioCtx.currentTime;
    const startAt = Math.max(now + 0.01, this.nextPlayTime);
    src.start(startAt);
    this.nextPlayTime = startAt + audioBuf.duration;
    this.activeSources.push(src);
    src.onended = () => {
      this.activeSources = this.activeSources.filter(s => s !== src);
      if (this.activeSources.length === 0) {
        if (this.agentSpeakingTimer) clearTimeout(this.agentSpeakingTimer);
        this.agentSpeakingTimer = setTimeout(() => {
          this.isAgentSpeaking = false;
          this.renderCard();
        }, 240);
      }
    };
  },

  clearPlayback() {
    this.activeSources.forEach(s => {
      try { s.stop(); } catch (_) {}
    });
    this.activeSources = [];
    this.nextPlayTime = 0;
    this.isAgentSpeaking = false;
    this.renderCard();
  },

  toggleMicMute() {
    this.isMicMuted = !this.isMicMuted;
    this.renderCard();
  },

  toggleSpeakerMute() {
    this.isSpeakerMuted = !this.isSpeakerMuted;
    if (this.gainNode) {
      this.gainNode.gain.value = this.isSpeakerMuted ? 0 : 1;
    }
    this.renderCard();
  },

  cycleSpeed() {
    const idx = this.supportedSpeeds.indexOf(this.playbackSpeed);
    this.playbackSpeed = this.supportedSpeeds[(idx + 1) % this.supportedSpeeds.length];
    this.sendSettingsUpdate(null, this.playbackSpeed);
    this.renderCard();
  },

  sendSettingsUpdate(planModelInt, speed) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return;
    const inner = [];
    if (planModelInt && planModelInt > 0) {
      inner.push(0x08);
      this.writeVarint(planModelInt, inner);
    }
    if (typeof speed === "number") {
      inner.push(0x11);
      const buf = new ArrayBuffer(8);
      new DataView(buf).setFloat64(0, speed, true);
      const arr = new Uint8Array(buf);
      for (let i = 0; i < 8; i++) inner.push(arr[i]);
    }
    if (!inner.length) return;
    const out = [0x1a];
    this.writeVarint(inner.length, out);
    const frame = new Uint8Array(out.concat(inner));
    this.ws.send(frame.buffer);
  },

  writeVarint(val, arr) {
    let v = val >>> 0;
    while (v >= 0x80) {
      arr.push((v & 0x7f) | 0x80);
      v >>>= 7;
    }
    arr.push(v);
  },

  readVarint(bytes, state) {
    let res = 0;
    let shift = 0;
    while (state.offset < bytes.length) {
      const b = bytes[state.offset++];
      res |= (b & 0x7f) << shift;
      if ((b & 0x80) === 0) return res >>> 0;
      shift += 7;
    }
    return null;
  },

  handleServerFrame(bytes) {
    const state = { offset: 0 };
    while (state.offset < bytes.length) {
      const tag = this.readVarint(bytes, state);
      if (tag === null) break;
      const field = tag >>> 3;
      const wt = tag & 0x07;
      if (wt === 0) {
        this.readVarint(bytes, state);
      } else if (wt === 1) {
        state.offset += 8;
      } else if (wt === 5) {
        state.offset += 4;
      } else if (wt === 2) {
        const len = this.readVarint(bytes, state);
        if (len === null || state.offset + len > bytes.length) return;
        const sub = bytes.subarray(state.offset, state.offset + len);
        state.offset += len;
        if (field === 1) {
          this.enqueuePcm24k(sub);
        } else if (field === 2) {
          this.handleStatus(sub);
        } else if (field === 7) {
          const text = this.extractFirstString(sub);
          if (text) this.showSubtitle(text, "ai");
        } else if (field === 8) {
          const text = this.extractFirstString(sub);
          if (text) this.showSubtitle(text, "user");
        } else if (field === 9) {
          this.clearPlayback();
        }
      } else {
        return;
      }
    }
  },

  handleStatus(sub) {
    const st = { offset: 0 };
    let stateVal = 0;
    while (st.offset < sub.length) {
      const tag = this.readVarint(sub, st);
      if (tag === null) break;
      const f = tag >>> 3;
      const wt = tag & 0x07;
      if (wt === 0) {
        const v = this.readVarint(sub, st);
        if (f === 1) stateVal = v;
      } else if (wt === 2) {
        const len = this.readVarint(sub, st);
        st.offset += (len || 0);
      } else if (wt === 1) {
        st.offset += 8;
      } else if (wt === 5) {
        st.offset += 4;
      } else {
        break;
      }
    }
    if (stateVal === 1 || stateVal === 2) {
      this.callState = "listening";
      this.renderCard();
    } else if (stateVal === 3) {
      this.callState = "reconnecting";
      this.renderCard();
    } else if (stateVal === 4) {
      this.endCall();
    } else if (stateVal === 5) {
      this.endCall();
      this.showTroubleshootBubble();
    }
  },

  extractFirstString(sub) {
    const st = { offset: 0 };
    while (st.offset < sub.length) {
      const tag = this.readVarint(sub, st);
      if (tag === null) break;
      const f = tag >>> 3;
      const wt = tag & 0x07;
      if (wt === 0) {
        this.readVarint(sub, st);
      } else if (wt === 2) {
        const len = this.readVarint(sub, st);
        if (len === null || st.offset + len > sub.length) return null;
        if (f === 1) {
          return new TextDecoder().decode(sub.subarray(st.offset, st.offset + len));
        }
        st.offset += len;
      } else if (wt === 1) {
        st.offset += 8;
      } else if (wt === 5) {
        st.offset += 4;
      } else {
        return null;
      }
    }
    return null;
  },

  showSubtitle(text, role) {
    const row = document.getElementById("voice-live-subtitle-row");
    const badge = document.getElementById("voice-subtitle-badge");
    const textEl = document.getElementById("voice-subtitle-text");
    if (!row || !badge || !textEl) return;
    badge.textContent = role === "ai" ? "AI" : "我";
    badge.classList.toggle("ai", role === "ai");
    textEl.textContent = text;
    row.classList.remove("hidden");
    if (this.subtitleTimer) clearTimeout(this.subtitleTimer);
    this.subtitleTimer = setTimeout(() => {
      row.classList.add("hidden");
    }, 3200);
  },

  pushWaveLevel(level) {
    this.soundLevels.shift();
    this.soundLevels.push(level);
    const bars = document.querySelectorAll("#voice-waveform-bars .voice-wave-bar");
    bars.forEach((bar, i) => {
      const lv = this.soundLevels[i] || 0.14;
      bar.style.height = `${Math.max(4, Math.round(lv * 24))}px`;
    });
  },

  updateTimerUI() {
    const timerEl = document.getElementById("voice-timer-text");
    if (!timerEl) return;
    if (this.callState === "connecting") {
      timerEl.textContent = "连接中";
      return;
    }
    if (this.callState === "reconnecting") {
      timerEl.textContent = "重连中";
      return;
    }
    const sec = Math.floor((Date.now() - this.startedAt) / 1000);
    const m = String(Math.floor(sec / 60)).padStart(2, "0");
    const s = String(sec % 60).padStart(2, "0");
    timerEl.textContent = `${m}:${s}`;
  },

  renderCard() {
    const card = document.getElementById("voice-call-card");
    if (!card) return;
    card.classList.toggle("hidden", !this.isInCall);
    card.classList.toggle("ai-speaking", this.isAgentSpeaking);
    card.classList.toggle("mic-muted", this.isMicMuted);

    const dot = document.getElementById("voice-status-dot");
    if (dot) {
      dot.className = "voice-status-dot " + (
        (this.callState === "connecting" || this.callState === "reconnecting")
          ? "connecting"
          : (this.isAgentSpeaking ? "ai" : "listening")
      );
    }
    const speedBtn = document.getElementById("btn-voice-speed");
    if (speedBtn) {
      speedBtn.textContent = `${this.playbackSpeed}x`;
    }
    const micBtn = document.getElementById("btn-voice-mic");
    if (micBtn) {
      micBtn.classList.toggle("muted-mic", this.isMicMuted);
    }
    const spkBtn = document.getElementById("btn-voice-speaker");
    if (spkBtn) {
      spkBtn.classList.toggle("muted-speaker", this.isSpeakerMuted);
    }
    this.updateTimerUI();
  }
};

// ==========================================================================
// Live Context Meter & Audit Sheet Module
// ==========================================================================

let currentContextMeter = null;
let currentContextIsRunning = false;
let currentContextHasPending = false;

function formatCompactTokensWeb(n) {
  const num = Number(n) || 0;
  if (num >= 1_000_000) return `${(num / 1_000_000).toFixed(1)}M`;
  if (num >= 1_000) return `${Math.round(num / 1_000)}K`;
  return String(num);
}

function getContextCacheDiagnostics(meter, isRunning = false, hasPendingInteraction = false) {
  if (!meter) {
    return { hitRate: null, isMissStreak: false, staleIdleMinutes: null, hasWarning: false };
  }
  let hitRate = null;
  if (typeof meter.cacheHitRate === "number" && !Number.isNaN(meter.cacheHitRate)) {
    hitRate = Math.min(Math.max(meter.cacheHitRate, 0), 100);
  } else {
    const inp = Math.max(Number(meter.inputTokens) || 0, 0);
    const cr = Math.max(Number(meter.cacheReadTokens) || 0, 0);
    if (inp + cr > 0) {
      hitRate = (cr / (inp + cr)) * 100;
    }
  }
  const isMissStreak = meter.cacheStatus === "miss_streak" || (Number(meter.cacheMissStreak) || 0) >= 2;
  let staleIdleMinutes = null;
  if (!isRunning && !hasPendingInteraction && (Number(meter.usedTokens) || 0) >= 40000 && meter.lastActivityAt) {
    const ts = Date.parse(meter.lastActivityAt);
    if (!Number.isNaN(ts)) {
      const mins = Math.floor((Date.now() - ts) / 60000);
      if (mins >= 15) staleIdleMinutes = mins;
    }
  }
  return {
    hitRate,
    isMissStreak,
    staleIdleMinutes,
    hasWarning: isMissStreak || staleIdleMinutes !== null,
  };
}

function formatCacheNoticeWeb(diag, meter) {
  const rawLang = (document.documentElement.getAttribute("data-lang") || navigator.language || "zh").toLowerCase();
  const lang = rawLang.startsWith("ja") ? "ja" : rawLang.startsWith("en") ? "en" : "zh";

  if (diag.isMissStreak) {
    const streak = Math.max(Number(meter.cacheMissStreak) || 2, 2);
    const uncached = formatCompactTokensWeb(Number(meter.cacheMissTokens) > 0 ? meter.cacheMissTokens : meter.inputTokens);
    if (lang === "en") {
      return `⚠ ${streak} consecutive cache misses (${uncached} uncached input) may drain weekly quota faster. Consider compacting or starting a new session.`;
    }
    if (lang === "ja") {
      return `⚠ 直近 ${streak} ステップ連続でキャッシュ未命中（再計算 ${uncached}）。週間クォータを急消費する恐れがあります。圧縮または新規チャットをおすすめします。`;
    }
    return `⚠ 近 ${streak} 步均未命中缓存（累计重算 ${uncached}），可能正在加速消耗周额度，建议压缩或开启新会话。`;
  }

  if (diag.staleIdleMinutes !== null) {
    const mins = diag.staleIdleMinutes;
    const h = Math.floor(mins / 60);
    const rem = mins % 60;
    const idleText =
      lang === "en"
        ? mins < 60 ? `${mins} min` : rem === 0 ? `${h}h` : `${h}h ${rem}m`
        : lang === "ja"
          ? mins < 60 ? `${mins} 分` : rem === 0 ? `${h} 時間` : `${h} 時間 ${rem} 分`
          : mins < 60 ? `${mins} 分钟` : rem === 0 ? `${h} 小时` : `${h} 小时 ${rem} 分钟`;
    const usedText = formatCompactTokensWeb(meter.usedTokens);
    if (lang === "en") {
      return `❄ Idle for ${idleText} — prompt cache for ~${usedText} context may have expired. Continuing may recompute full input; consider compacting or starting a new session.`;
    }
    if (lang === "ja") {
      return `❄ ${idleText} 間アイドル状態です。約 ${usedText} のキャッシュが失効している可能性があります。このまま続行すると入力を再計算するため、圧縮または新規チャットをおすすめします。`;
    }
    return `❄ 已闲置 ${idleText}，约 ${usedText} 上下文缓存可能已失效；直接续聊将重算输入额度，建议先压缩或开启新会话。`;
  }

  const pct = Math.min(Math.max(Number(meter.percentage) || 0, 0), 100);
  const pctInt = Math.round(pct);
  if (meter.isOverflow || pct >= 95) {
    if (lang === "en") {
      return `⚠ Context window is nearly full (${pctInt}%) — auto-compaction or truncation may trigger at any time. Consider compacting now or starting a new session.`;
    }
    if (lang === "ja") {
      return `⚠ コンテキスト上限間近（${pctInt}%）です。自動圧縮や切り詰めが発生する恐れがあるため、今すぐ圧縮するか新規チャットをおすすめします。`;
    }
    return `⚠ 上下文窗口已接近上限（${pctInt}%），随时可能触发自动压缩或截断，建议先一键压缩保留关键上下文，或开启新会话。`;
  }
  if (pct >= 75) {
    if (lang === "en") {
      return `ℹ Context is at ${pctInt}%, approaching auto-compaction. Compacting before the next long step helps avoid redundant input cost.`;
    }
    if (lang === "ja") {
      return `ℹ コンテキスト使用率が ${pctInt}% に達し、自動圧縮のしきい値に近づいています。長い作業の前に圧縮すると再入力コストを抑えられます。`;
    }
    return `ℹ 上下文已用 ${pctInt}%，临近自动压缩阈值；继续长任务前建议先一键压缩，减少后续重复输入消耗。`;
  }
  return "";
}

function formatSessionTotalsHeaderWeb(steps) {
  const rawLang = (document.documentElement.getAttribute("data-lang") || navigator.language || "zh").toLowerCase();
  const lang = rawLang.startsWith("ja") ? "ja" : rawLang.startsWith("en") ? "en" : "zh";
  if (lang === "en") {
    return steps > 0 ? `Session Total · ${steps} steps` : "Session Total";
  }
  if (lang === "ja") {
    return steps > 0 ? `セッション累計 · ${steps} ステップ` : "セッション累計";
  }
  return steps > 0 ? `本会话累计 · ${steps} 步` : "本会话累计";
}

function updateContextMeterUI(meter, isRunning, hasPendingInteraction) {
  currentContextMeter = meter;
  if (typeof isRunning === "boolean") currentContextIsRunning = isRunning;
  if (typeof hasPendingInteraction === "boolean") currentContextHasPending = hasPendingInteraction;
  const ringBtn = document.getElementById("chat-context-ring-btn");
  const ringCircle = document.getElementById("context-ring-circle");
  const ringText = document.getElementById("context-ring-text");
  const cacheBadge = document.getElementById("context-cache-badge");
  const speedBadge = document.getElementById("context-speed-badge");

  if (!ringBtn || !meter) {
    if (ringBtn) ringBtn.classList.add("hidden");
    return;
  }

  const diag = getContextCacheDiagnostics(meter, currentContextIsRunning, currentContextHasPending);
  const shouldShow = window.CompactModePrefs
    ? window.CompactModePrefs.shouldShowContextMeter(meter, currentContextIsRunning, currentContextHasPending)
    : true;
  if (!shouldShow) {
    ringBtn.classList.add("hidden");
  } else {
    ringBtn.classList.remove("hidden");
  }
  const pct = Math.min(Math.max(meter.percentage || 0, 0), 100);
  if (ringCircle) {
    ringCircle.setAttribute("stroke-dasharray", `${pct.toFixed(1)}, 100`);
    ringCircle.classList.remove("warn", "danger");
    if (pct >= 90 || meter.isOverflow) {
      ringCircle.classList.add("danger");
    } else if (pct >= 70 || diag.hasWarning) {
      ringCircle.classList.add("warn");
    }
  }
  if (ringText) {
    ringText.textContent = `${Math.round(pct)}%`;
  }
  if (cacheBadge) {
    if (diag.isMissStreak) {
      cacheBadge.textContent = "⚠";
      cacheBadge.classList.remove("hidden");
    } else if (diag.staleIdleMinutes !== null) {
      cacheBadge.textContent = "❄";
      cacheBadge.classList.remove("hidden");
    } else {
      cacheBadge.textContent = "";
      cacheBadge.classList.add("hidden");
    }
  }
  if (speedBadge) {
    if ((currentContextIsRunning || meter.tokensPerSecond > 0) && meter.tokensPerSecond > 0) {
      speedBadge.textContent = `⚡ ${meter.tokensPerSecond.toFixed(1)} t/s`;
      speedBadge.classList.remove("hidden");
    } else {
      speedBadge.classList.add("hidden");
    }
  }

  // If audit modal is open, keep it fresh
  const sheet = document.getElementById("sheet-audit");
  if (sheet && !sheet.classList.contains("hidden")) {
    renderAuditSheetContent(meter);
  }
}

function renderAuditSheetContent(meter) {
  if (!meter) return;
  const usedEl = document.getElementById("audit-used-tokens");
  const maxEl = document.getElementById("audit-max-tokens");
  const fillEl = document.getElementById("audit-progress-fill");
  const ttftEl = document.getElementById("audit-ttft-val");
  const tpsEl = document.getElementById("audit-tps-val");
  const durEl = document.getElementById("audit-stream-duration");
  const inpEl = document.getElementById("audit-input-tokens");
  const capEl = document.getElementById("audit-cache-tokens");
  const outEl = document.getElementById("audit-output-tokens");
  const thinkEl = document.getElementById("audit-thinking-tokens");
  const noticeEl = document.getElementById("audit-cache-notice");

  const diag = getContextCacheDiagnostics(meter, currentContextIsRunning, currentContextHasPending);
  const pct = Math.min(Math.max(meter.percentage || 0, 0), 100);
  if (usedEl) usedEl.textContent = (meter.usedTokens || 0).toLocaleString();
  if (maxEl) maxEl.textContent = meter.maxTokens >= 1000 ? `${Math.round(meter.maxTokens / 1000)}k` : String(meter.maxTokens || 200000);
  if (fillEl) {
    fillEl.style.width = `${pct}%`;
    fillEl.style.background =
      pct >= 90 || meter.isOverflow
        ? "var(--ios-red, #ff3b30)"
        : pct >= 70 || diag.hasWarning
          ? "var(--ios-orange, #ff9500)"
          : "var(--ios-green, #34c759)";
  }
  if (ttftEl) ttftEl.textContent = meter.timeToFirstTokenSec > 0 ? meter.timeToFirstTokenSec.toFixed(2) : "--";
  if (tpsEl) tpsEl.textContent = meter.tokensPerSecond > 0 ? meter.tokensPerSecond.toFixed(1) : "--";
  if (durEl) durEl.textContent = meter.streamingDurationSec > 0 ? meter.streamingDurationSec.toFixed(2) : "--";

  if (inpEl) inpEl.textContent = (meter.inputTokens || 0).toLocaleString();
  if (capEl) {
    const baseText = (meter.cacheReadTokens || 0).toLocaleString();
    capEl.textContent = diag.hitRate !== null ? `${baseText} (${Math.round(diag.hitRate)}%)` : baseText;
    const lowHit = diag.isMissStreak || (diag.hitRate !== null && (Number(meter.usedTokens) || 0) >= 24000 && diag.hitRate < 20);
    capEl.classList.toggle("audit-val-warn", lowHit);
  }
  if (outEl) outEl.textContent = (meter.outputTokens || 0).toLocaleString();
  if (thinkEl) thinkEl.textContent = (meter.thinkingTokens || 0).toLocaleString();

  const sessionSection = document.getElementById("audit-session-section");
  if (sessionSection) {
    const sessSteps = Number(meter.sessionSteps) || 0;
    const sessInp = Number(meter.sessionInputTokens) || 0;
    const sessCache = Number(meter.sessionCacheReadTokens) || 0;
    const sessOut = Number(meter.sessionOutputTokens) || 0;
    const sessThink = Number(meter.sessionThinkingTokens) || 0;
    const hasSessionTotals = sessSteps > 0 || sessInp + sessCache + sessOut + sessThink > 0;
    if (hasSessionTotals) {
      sessionSection.classList.remove("hidden");
      const titleEl = document.getElementById("audit-session-title");
      const costEl = document.getElementById("audit-session-cost");
      const sInpEl = document.getElementById("audit-session-input");
      const sCacheEl = document.getElementById("audit-session-cache");
      const sOutEl = document.getElementById("audit-session-output");
      const sThinkEl = document.getElementById("audit-session-thinking");
      if (titleEl) titleEl.textContent = formatSessionTotalsHeaderWeb(sessSteps);
      if (costEl) {
        const cost = Number(meter.sessionCostUsd) || 0;
        if (cost >= 0.01) {
          costEl.textContent = `≈ $${cost.toFixed(2)}`;
          costEl.classList.remove("hidden");
        } else {
          costEl.textContent = "";
          costEl.classList.add("hidden");
        }
      }
      if (sInpEl) sInpEl.textContent = sessInp.toLocaleString();
      if (sCacheEl) {
        let sessHitRate = null;
        if (typeof meter.sessionCacheHitRate === "number" && Number.isFinite(meter.sessionCacheHitRate)) {
          sessHitRate = Math.min(Math.max(meter.sessionCacheHitRate, 0), 100);
        } else if (sessInp + sessCache > 0) {
          sessHitRate = (sessCache / (sessInp + sessCache)) * 100;
        }
        const baseText = sessCache.toLocaleString();
        sCacheEl.textContent = sessHitRate !== null ? `${baseText} (${Math.round(sessHitRate)}%)` : baseText;
      }
      if (sOutEl) sOutEl.textContent = sessOut.toLocaleString();
      if (sThinkEl) sThinkEl.textContent = sessThink.toLocaleString();
    } else {
      sessionSection.classList.add("hidden");
    }
  }

  if (noticeEl) {
    const noticeText = formatCacheNoticeWeb(diag, meter);
    if (noticeText) {
      noticeEl.textContent = noticeText;
      noticeEl.classList.remove("hidden");
    } else {
      noticeEl.textContent = "";
      noticeEl.classList.add("hidden");
    }
  }
}

function openAuditSheet() {
  const sheet = document.getElementById("sheet-audit");
  if (!sheet) return;
  if (currentContextMeter) {
    renderAuditSheetContent(currentContextMeter);
  }
  sheet.classList.remove("hidden");
}

function closeAuditSheet() {
  const sheet = document.getElementById("sheet-audit");
  if (sheet) sheet.classList.add("hidden");
}

function sendCompactCommand() {
  closeAuditSheet();
  const inputEl = document.getElementById("chat-input");
  if (inputEl) {
    inputEl.value = "/compact";
    if (typeof sendMessage === "function") {
      sendMessage();
    }
  }
}

document.addEventListener("DOMContentLoaded", () => {
  const ringBtn = document.getElementById("chat-context-ring-btn");
  if (ringBtn) ringBtn.addEventListener("click", openAuditSheet);

  const closeBtn = document.getElementById("btn-audit-close");
  if (closeBtn) closeBtn.addEventListener("click", closeAuditSheet);

  const compactBtn = document.getElementById("btn-audit-compact");
  if (compactBtn) compactBtn.addEventListener("click", sendCompactCommand);

  const sheet = document.getElementById("sheet-audit");
  if (sheet) {
    sheet.addEventListener("click", (e) => {
      if (e.target === sheet) closeAuditSheet();
    });
  }
});

