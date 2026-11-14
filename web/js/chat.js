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

function checkLatestMessageIsError(steps, isRunning) {
  if (isRunning) return false;
  if (!steps || steps.length === 0) return false;
  const items = groupSteps(steps);
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
let autoApprovePermissions = localStorage.getItem("agy_auto_approve_permissions") === "true";
let currentPendingInteraction = null;
let selectedInteractionOptionId = null;
let isSubmittingInteraction = false;

function updatePendingInteraction(interaction, isRunning) {
  const container = document.getElementById("interaction-card-container");
  if (!container) return;

  if (!isRunning || !interaction || !interaction.options || interaction.options.length === 0) {
    currentPendingInteraction = null;
    selectedInteractionOptionId = null;
    container.innerHTML = "";
    container.classList.add("hidden");
    return;
  }

  const isDifferent = !currentPendingInteraction ||
    currentPendingInteraction.stepIndex !== interaction.stepIndex ||
    currentPendingInteraction.type !== interaction.type;

  currentPendingInteraction = interaction;
  if (isDifferent || !selectedInteractionOptionId) {
    const isPermissionType = interaction.type === "permission" || interaction.type === "file_permission";
    if (autoApprovePermissions && isPermissionType) {
      const opt4 = interaction.options.find(o => o.scope === 4 || o.id === "4" || (o.text && o.text.toLowerCase().includes("always allow")));
      selectedInteractionOptionId = opt4 ? opt4.id : (interaction.options[0]?.id || "");
    } else {
      selectedInteractionOptionId = interaction.options[0]?.id || "";
    }
  }

  renderInteractionCard();

  // Trigger auto-approve if enabled and this is a permission request
  const isPermissionType = interaction.type === "permission" || interaction.type === "file_permission";
  if (autoApprovePermissions && isPermissionType && !isSubmittingInteraction) {
    const opt4 = interaction.options.find(o => o.scope === 4 || o.id === "4" || (o.text && o.text.toLowerCase().includes("always allow")));
    if (opt4) {
      selectedInteractionOptionId = opt4.id;
      setTimeout(() => {
        if (currentPendingInteraction && currentPendingInteraction.stepIndex === interaction.stepIndex && !isSubmittingInteraction) {
          handleInteractionSubmit(false);
        }
      }, 250);
    }
  }
}

let multiQuestionSelections = {};
let multiQuestionWriteIns = {};
let singleWriteInText = "";

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

function renderInteractionCard() {
  const container = document.getElementById("interaction-card-container");
  if (!container || !currentPendingInteraction) return;

  const interaction = currentPendingInteraction;
  const isPermission = interaction.type === "permission" || interaction.type === "file_permission";
  const isDownload = interaction.type === "browser_download";
  const isApproval = interaction.type === "approval" || isDownload;
  const hasMultiQuestions = interaction.questions && interaction.questions.length > 1;

  let iconSvg = '';
  if (isDownload) {
    iconSvg = `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="#f59e0b" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path><polyline points="7 10 12 15 17 10"></polyline><line x1="12" y1="15" x2="12" y2="3"></line></svg>`;
  } else if (isPermission) {
    iconSvg = `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"></path></svg>`;
  } else {
    iconSvg = `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"></path><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>`;
  }

  let contentHtml = '';
  if (hasMultiQuestions) {
    contentHtml = `
      <div class="interaction-multi-questions-list" style="max-height: 280px; overflow-y: auto; display: flex; flex-direction: column; gap: 12px; margin-bottom: 8px; padding-right: 4px;">
        ${interaction.questions.map((q, qIdx) => {
          const hasWriteIn = q.hasWriteIn && !(q.options || []).some(o => o.id === '__write_in__' || o.isDeny);
          const writeInLabel = resolveWriteInLabel(q.writeInLabel);
          const opts = hasWriteIn
            ? [...(q.options || []), { id: '__write_in__', text: writeInLabel }]
            : (q.options || []);
          const selectedId = multiQuestionSelections[qIdx] || q.defaultOptionId || opts[0]?.id || "1";
          const isDenySelected = selectedId === "5" || selectedId === "__write_in__" || selectedId.toLowerCase() === "other";
          const isNumeric = (q.options || []).length > 0 && (q.options || []).every(o => !isNaN(parseInt(o.id, 10)));
          return `
            <div class="interaction-question-block" style="border: 1px solid var(--border-color, rgba(255,255,255,0.08)); border-radius: 8px; padding: 10px; background: rgba(0,0,0,0.15);">
              <div style="display: flex; align-items: flex-start; gap: 6px; margin-bottom: 8px;">
                <span style="background: #9333ea; color: white; font-size: 10px; font-weight: bold; padding: 2px 5px; border-radius: 4px; font-family: monospace;">Q${qIdx + 1}</span>
                <span style="font-size: 13px; font-weight: 600; color: var(--text-primary); line-height: 1.3;">${escapeHtml(q.question)}</span>
              </div>
              <div class="interaction-options-list" style="display: flex; flex-direction: column; gap: 5px;">
                ${opts.map((opt, optIdx) => {
                  const isSelected = opt.id === selectedId;
                  const label = opt.text || opt.label || "";
                  const badge = opt.id === '__write_in__' ? (isNumeric ? String(optIdx + 1) : '✎') : (opt.id || String(optIdx + 1));
                  const isWriteIn = opt.id === '__write_in__' || (q.hasWriteIn && (opt.isDeny || opt.id === '5' || opt.id.toLowerCase() === 'other'));
                  return `
                    <div class="interaction-option-item ${isSelected ? 'selected' : ''}" data-q-idx="${qIdx}" data-opt-id="${escapeHtml(opt.id)}" style="display: flex; align-items: center; gap: 8px; padding: 6px 9px; border-radius: 6px; cursor: pointer; background: ${isSelected ? 'rgba(59,130,246,0.12)' : 'var(--bg-tertiary, rgba(255,255,255,0.04))'}; border: 1px solid ${isSelected ? '#3b82f6' : 'transparent'};">
                      <span class="interaction-option-radio" style="width: 14px; height: 14px; border-radius: 50%; border: 1.5px solid ${isSelected ? '#3b82f6' : 'var(--text-secondary)'}; display: flex; align-items: center; justify-content: center; flex-shrink: 0;">
                        ${isSelected ? '<span style="width: 6px; height: 6px; border-radius: 50%; background: #3b82f6;"></span>' : ''}
                      </span>
                      <span class="interaction-option-badge" style="font-size: 11px; font-weight: bold; font-family: monospace; color: ${isSelected ? '#3b82f6' : 'var(--text-secondary)'}; flex-shrink: 0;">[${escapeHtml(badge)}]</span>
                      ${isWriteIn && isSelected ? `
                        <input type="text" class="interaction-multi-write-in-input" data-q-idx="${qIdx}" placeholder="${escapeHtml(resolveWriteInPlaceholder(q.writeInPlaceholder, false))}" value="${escapeHtml(multiQuestionWriteIns[qIdx] || '')}" style="flex: 1; font-size: 12.5px; border: none; background: transparent; color: var(--text-primary); outline: none; padding: 0;" />
                      ` : `
                        <span class="interaction-option-label" style="font-size: 12.5px; color: ${isSelected ? 'var(--text-primary)' : 'var(--text-secondary)'}; overflow-x: auto; white-space: nowrap; flex: 1; scrollbar-width: none;">${escapeHtml(isWriteIn && multiQuestionWriteIns[qIdx] ? multiQuestionWriteIns[qIdx] : label)}</span>
                      `}
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
    const isDenyOrWriteIn = selectedOpt?.isDeny || selectedInteractionOptionId === "5" || selectedInteractionOptionId === "__write_in__" || selectedOpt?.id === "no" || selectedOpt?.id?.toLowerCase().includes("deny");
    const isNumeric = (interaction.options || []).length > 0 && (interaction.options || []).every(o => !isNaN(parseInt(o.id, 10)));

    contentHtml = `
      <div class="interaction-options-list" style="display: flex; flex-direction: column; gap: 6px;">
        ${displayOptions.map((opt, idx) => {
          const isSelected = opt.id === selectedInteractionOptionId;
          const label = opt.text || opt.label || "";
          const badge = opt.id === '__write_in__' ? (isNumeric ? String(idx + 1) : '✎') : (opt.id || String(idx + 1));
          const isWriteIn = opt.id === '__write_in__' || (interaction.hasWriteIn && (opt.isDeny || opt.id === '5'));
          return `
            <div class="interaction-option-item ${isSelected ? 'selected' : ''}" data-opt-id="${escapeHtml(opt.id)}" style="display: flex; align-items: center; gap: 8px; padding: 8px 10px; border-radius: 8px; cursor: pointer; background: ${isSelected ? 'rgba(59,130,246,0.12)' : 'var(--bg-tertiary, rgba(255,255,255,0.04))'}; border: 1px solid ${isSelected ? '#3b82f6' : 'transparent'};">
              <span class="interaction-option-radio" style="width: 16px; height: 16px; border-radius: 50%; border: 1.5px solid ${isSelected ? '#3b82f6' : 'var(--text-secondary)'}; display: flex; align-items: center; justify-content: center; flex-shrink: 0;">
                ${isSelected ? '<span style="width: 8px; height: 8px; border-radius: 50%; background: #3b82f6;"></span>' : ''}
              </span>
              <span class="interaction-option-badge" style="font-size: 11px; font-weight: bold; font-family: monospace; color: ${isSelected ? '#3b82f6' : 'var(--text-secondary)'}; flex-shrink: 0;">[${escapeHtml(badge)}]</span>
              ${isWriteIn && isSelected ? `
                <input type="text" id="interaction-write-in-input" class="interaction-write-in-input" placeholder="${escapeHtml(resolveWriteInPlaceholder(interaction.writeInPlaceholder, isDenyOrWriteIn))}" value="${escapeHtml(singleWriteInText)}" style="flex: 1; font-size: 13px; border: none; background: transparent; color: var(--text-primary); outline: none; padding: 0;" />
              ` : `
                <span class="interaction-option-label" style="font-size: 13px; color: ${isSelected ? 'var(--text-primary)' : 'var(--text-secondary)'}; overflow-x: auto; white-space: nowrap; flex: 1; scrollbar-width: none;">${escapeHtml(isWriteIn && singleWriteInText ? singleWriteInText : label)}</span>
              `}
            </div>
          `;
        }).join('')}
      </div>
    `;
  }

  container.innerHTML = `
    <div class="interaction-card" style="padding: 14px; border-radius: 12px; border: 1px solid rgba(59,130,246,0.35); background: var(--bg-secondary, #1a1d24); box-shadow: 0 4px 16px rgba(0,0,0,0.25);">
      <div class="interaction-card-header" style="display: flex; align-items: center; gap: 8px; margin-bottom: 10px;">
        <div class="interaction-icon" style="color: ${hasMultiQuestions ? '#9333ea' : '#3b82f6'};">${iconSvg}</div>
        <div class="interaction-title-group" style="flex: 1;">
          <div class="interaction-title" style="font-size: 14px; font-weight: bold; color: var(--text-primary);">
            ${hasMultiQuestions ? `需要确认规格 (${interaction.questions.length} 个问题)` : escapeHtml(interaction.title || "需要确认或授权")}
          </div>
        </div>
      </div>

      ${interaction.target ? `
        <div class="interaction-target-box" style="margin-bottom: 10px; padding: 6px 10px; border-radius: 6px; background: rgba(0,0,0,0.2); font-family: monospace; font-size: 11.5px; color: var(--text-secondary); word-break: break-all;">
          ${escapeHtml(interaction.target)}
        </div>
      ` : ''}

      ${contentHtml}

      <div class="interaction-card-actions" style="display: flex; align-items: center; gap: 10px; margin-top: 12px;">
        ${(interaction.type === "permission" || interaction.type === "file_permission") ? `
          <button type="button" id="btn-interaction-auto-approve" class="btn-interaction-auto ${autoApprovePermissions ? 'active' : ''}" style="border: 1px solid var(--border-color); border-radius: 6px; padding: 4px 8px; font-size: 11px; background: ${autoApprovePermissions ? 'rgba(234, 179, 8, 0.15)' : 'transparent'}; color: ${autoApprovePermissions ? '#ca8a04' : 'var(--text-secondary)'}; cursor: pointer;">
            <span>⚡️ ${autoApprovePermissions ? '自动审批: 开' : '自动审批: 关'}</span>
          </button>
        ` : ''}
        <button type="button" id="btn-interaction-skip" class="btn-interaction-skip" ${isSubmittingInteraction ? 'disabled' : ''} style="flex: 1; text-align: center; padding: 8px 14px; border-radius: 8px; border: 1px solid var(--border-color); background: transparent; color: var(--text-secondary); cursor: pointer;">跳过</button>
        <button type="button" id="btn-interaction-submit" class="btn-interaction-submit" ${isSubmittingInteraction ? 'disabled' : ''} style="flex: 1; text-align: center; padding: 8px 16px; border-radius: 8px; border: none; background: #2563eb; color: white; font-weight: 600; cursor: pointer;">
          <span>${isSubmittingInteraction ? '提交中...' : '提交'}</span>
        </button>
      </div>
    </div>
  `;

  container.classList.remove("hidden");

  // Multi-question option click handlers
  container.querySelectorAll(".interaction-option-item[data-q-idx]").forEach(item => {
    item.addEventListener("click", (e) => {
      if (e.target && e.target.classList.contains("interaction-multi-write-in-input")) {
        return;
      }
      const qIdx = parseInt(item.dataset.qIdx, 10);
      const optId = item.dataset.optId;
      if (optId) {
        multiQuestionSelections[qIdx] = optId;
        renderInteractionCard();
        if (optId === "__write_in__" || optId === "5") {
          setTimeout(() => {
            const input = container.querySelector(`.interaction-multi-write-in-input[data-q-idx="${qIdx}"]`);
            if (input) {
              input.focus();
              input.selectionStart = input.selectionEnd = input.value.length;
            }
          }, 30);
        }
      }
    });
  });

  // Multi-question write-in inputs
  container.querySelectorAll(".interaction-multi-write-in-input").forEach(input => {
    input.addEventListener("input", (e) => {
      const qIdx = parseInt(input.dataset.qIdx, 10);
      multiQuestionWriteIns[qIdx] = e.target.value;
    });
  });

  // Single-question option click handlers
  container.querySelectorAll(".interaction-option-item:not([data-q-idx])").forEach(item => {
    item.addEventListener("click", (e) => {
      if (e.target && e.target.id === "interaction-write-in-input") {
        return;
      }
      const optId = item.dataset.optId;
      if (optId) {
        selectedInteractionOptionId = optId;
        renderInteractionCard();
        if (optId === "__write_in__" || optId === "5") {
          setTimeout(() => {
            const input = container.querySelector("#interaction-write-in-input");
            if (input) {
              input.focus();
              input.selectionStart = input.selectionEnd = input.value.length;
            }
          }, 30);
        }
      }
    });
  });

  const autoApproveBtn = container.querySelector("#btn-interaction-auto-approve");
  if (autoApproveBtn) {
    autoApproveBtn.addEventListener("click", () => {
      autoApprovePermissions = !autoApprovePermissions;
      localStorage.setItem("agy_auto_approve_permissions", autoApprovePermissions ? "true" : "false");
      if (autoApprovePermissions && currentPendingInteraction) {
        const opt4 = currentPendingInteraction.options?.find(o => o.scope === 4 || o.id === "4" || (o.text && o.text.toLowerCase().includes("always allow")));
        if (opt4) {
          selectedInteractionOptionId = opt4.id;
        }
      }
      renderInteractionCard();
    });
  }

  const writeInInput = container.querySelector("#interaction-write-in-input");
  if (writeInInput) {
    writeInInput.addEventListener("input", (e) => {
      singleWriteInText = e.target.value;
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

  isSubmittingInteraction = true;
  renderInteractionCard();

  try {
    const interaction = currentPendingInteraction;
    const hasMultiQuestions = interaction.questions && interaction.questions.length > 1;

    let questionResponses = null;
    let selectedOptionId = isSkip ? "" : (selectedInteractionOptionId || "");
    let writeInText = "";

    if (hasMultiQuestions) {
      questionResponses = interaction.questions.map((q, idx) => {
        const optId = multiQuestionSelections[idx] || q.defaultOptionId || q.options?.[0]?.id || "1";
        const writeIn = multiQuestionWriteIns[idx] || "";
        return {
          questionIndex: idx,
          selectedOptionIds: isSkip ? [] : [optId],
          writeInResponse: isSkip ? "" : writeIn,
          skipped: isSkip
        };
      });
      selectedOptionId = questionResponses[0]?.selectedOptionIds?.[0] || "";
    } else {
      const writeInInput = document.getElementById("interaction-write-in-input");
      writeInText = writeInInput ? writeInInput.value.trim() : "";
    }

    const selectedOpt = interaction.options?.find(o => o.id === selectedOptionId);
    const isDeny = selectedOpt?.isDeny || selectedOptionId === "5" || selectedOptionId === "__write_in__";

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
    multiQuestionSelections = {};
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
    alert("提交失败: " + err.message);
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

function getVisibleTurnIndex(userRows, streamEl) {
  if (!userRows || userRows.length === 0) return 0;
  const scrollTop = streamEl.scrollTop;
  let bestIdx = 0;
  for (let i = 0; i < userRows.length; i++) {
    const rowTop = userRows[i].offsetTop - streamEl.offsetTop;
    if (rowTop <= scrollTop + 60) {
      bestIdx = i;
    } else {
      break;
    }
  }
  return bestIdx;
}

function updatePromptJumpStepperStateOnly() {
  const stepperEl = document.getElementById("prompt-jump-stepper");
  const streamEl = document.getElementById("messages-stream");
  if (!stepperEl || !streamEl) return;

  const userRows = Array.from(streamEl.querySelectorAll(".message-row.user"));
  const totalTurns = userRows.length;
  if (totalTurns < 2) return;

  const currentIdx = getVisibleTurnIndex(userRows, streamEl);
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

function updatePromptJumpStepper() {
  const stepperEl = document.getElementById("prompt-jump-stepper");
  const streamEl = document.getElementById("messages-stream");
  const chatInput = document.getElementById("chat-input");
  if (!stepperEl || !streamEl) return;

  initPromptJumpStepper();

  const userRows = Array.from(streamEl.querySelectorAll(".message-row.user"));
  const totalTurns = userRows.length;

  const isInputFocused = chatInput && document.activeElement === chatInput;
  const dist = streamEl.scrollHeight - streamEl.scrollTop - streamEl.clientHeight;
  const isNearBottom = dist <= 120;

  if (totalTurns < 2 || isNearBottom || isInputFocused) {
    hidePromptJumpStepperImmediately();
    return;
  }

  updatePromptJumpStepperStateOnly();
}

function initScrollListener() {
  const streamEl = document.getElementById("messages-stream");
  if (!streamEl || streamEl.dataset.hasScrollListener) return;
  streamEl.dataset.hasScrollListener = "true";

  initPromptJumpStepper();

  const updateNearBottom = () => {
    const dist = streamEl.scrollHeight - streamEl.scrollTop - streamEl.clientHeight;
    userIsNearBottom = dist <= 80;
    if (userIsNearBottom) {
      hidePromptJumpStepperImmediately();
    }
  };

  streamEl.addEventListener("scroll", () => {
    updateNearBottom();
    const currentScrollTop = streamEl.scrollTop;
    const deltaY = currentScrollTop - pjsLastScrollTop;
    pjsLastScrollTop = currentScrollTop;

    const userRows = streamEl.querySelectorAll(".message-row.user");
    if (userRows.length < 2 || userIsNearBottom) {
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
    updatePromptJumpStepperStateOnly();
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
      wsText.textContent = wsName;
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
    const hasContent = (chatInput && chatInput.value.trim().length > 0) || (pendingImages && pendingImages.length > 0) || !!getSelectedSlashCommand();
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
      sendBtn.title = "挂断实时语音通话";
      sendBtn.setAttribute("aria-label", "挂断实时语音通话");
      showOnlyIcon(iconHangup);
    } else if (hasContent || (!isRunning && !voiceEnabled)) {
      sendBtn.className = "btn-action-circle send-mode" + (hasContent ? " active" : "");
      sendBtn.title = isRunning ? "加入待发送队列" : "发送";
      sendBtn.setAttribute("aria-label", isRunning ? "加入待发送队列" : "发送");
      showOnlyIcon(iconSend);
    } else if (isRunning) {
      sendBtn.className = "btn-action-circle stop-mode";
      sendBtn.title = "停止任务";
      sendBtn.setAttribute("aria-label", "停止任务");
      showOnlyIcon(iconStop);
    } else {
      sendBtn.className = "btn-action-circle voice-mode";
      sendBtn.title = "实时语音对话";
      sendBtn.setAttribute("aria-label", "实时语音对话");
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

async function connectStreamWs(cascadeId) {
  if (activeWs && activeWs.__cascadeId === cascadeId && (activeWs.readyState === WebSocket.OPEN || activeWs.readyState === WebSocket.CONNECTING)) {
    return;
  }
  const mySeq = ++streamWsSeq;
  closeActiveWs();

  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  let wsUrl = `${proto}//${location.host}/gateway/cascade/stream?cascadeId=${encodeURIComponent(cascadeId)}&delta=1&messages=0`;
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

        if (!applyStreamDelta(ws, data)) {
          // Baseline mismatch: drop this connection and let onclose reconnect for a fresh "init".
          console.warn("[WS] Delta baseline mismatch, resyncing");
          ws.close();
          return;
        }

        if (data.activeModel) {
          syncActiveModel(data.activeModel);
        }

        const isRunning = data.status === "CASCADE_RUN_STATUS_RUNNING" || hasActiveRunningTasks(data.runningTasks);

        if (data.contextMeter && typeof updateContextMeterUI === "function") {
          updateContextMeterUI(data.contextMeter, isRunning);
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
        if (typeof SwarmManager !== "undefined") SwarmManager.sync(data);

        if (data.steps) {
          scheduleRenderMessages(data.steps, isRunning);
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
    LocalQueueManager.init(cascadeId);
    RunningTasksManager.init(cascadeId);
    renderMessages(steps, isRunning);

    // P1 / B-4: Avoid redundant HTTP messages request when WebSocket stream is active
    if (!activeWs || activeWs.readyState !== WebSocket.OPEN) {
      fetch(`/gateway/cascade/messages?cascadeId=${encodeURIComponent(cascadeId)}&limit=1`)
        .then(res => res.json())
        .then(info => {
          if (activeCascadeId === cascadeId) {
            if (info.activeModel) {
              syncActiveModel(info.activeModel);
            }
            if (info.queuedMessages !== undefined) {
              LocalQueueManager.syncFromServer(info.queuedMessages, info.messages || info.steps);
            } else if (info.messages) {
              LocalQueueManager.syncFromServer(null, info.messages);
            }
            RunningTasksManager.syncFromServer(info.runningTasks);
            SubagentManager.sync(info);
            if (currentTrajectories[cascadeId]) {
              if (info.status) currentTrajectories[cascadeId].status = info.status;
              currentTrajectories[cascadeId].runningTasks = Array.isArray(info.runningTasks) ? info.runningTasks : [];
            }
            const liveRunning = isCascadeRunning(cascadeId);
            if (info.contextMeter && typeof updateContextMeterUI === "function") {
              updateContextMeterUI(info.contextMeter, liveRunning);
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

function groupSteps(steps) {
  if (!steps || !steps.length) return [];

  const items = [];
  let currentBatch = null;

  function flushBatch() {
    if (currentBatch && currentBatch.steps.length > 0) {
      items.push(currentBatch);
      currentBatch = null;
    }
  }

  for (let i = 0; i < steps.length; i++) {
    const s = steps[i];
    const type = s.type || "";

    if (type === "CORTEX_STEP_TYPE_USER_INPUT") {
      flushBatch();
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
    } else if (type === "CORTEX_STEP_TYPE_PLANNER_RESPONSE") {
      const p = s.plannerResponse || {};
      const rawResp = (p.response || "").trim();
      const resp = cleanInternalSystemMessage(rawResp);
      const thinking = (p.thinking || "").trim();

      if (resp) {
        flushBatch();
        const artList = s.artifacts ? [...s.artifacts] : [];
        items.push({
          type: "agent",
          id: `item-agent-${i}`,
          index: i,
          text: resp,
          thinking: thinking,
          artifacts: artList,
          step: s
        });
      } else if (thinking) {
        if (!currentBatch) {
          currentBatch = {
            type: "tools",
            id: `item-tools-${i}`,
            startIndex: i,
            steps: [],
            toolNames: []
          };
        }
        currentBatch.steps.push({
          name: "thinking",
          detail: thinking.length > 60 ? thinking.slice(0, 57) + "..." : thinking,
          status: "DONE",
          raw: s
        });
        if (!currentBatch.toolNames.includes("thinking") && currentBatch.toolNames.length < 3) {
          currentBatch.toolNames.push("thinking");
        }
      }
    } else if (type === "CORTEX_STEP_TYPE_INVOKE_SUBAGENT") {
      // 与桌面端一致：在调用位置内联展示每个子代理的卡片，而不是折进「已执行 N 项操作」
      flushBatch();
      const inv = s.invokeSubagent || {};
      const specs = Array.isArray(inv.subagents) ? inv.subagents : [];
      (Array.isArray(inv.results) ? inv.results : []).forEach((res, k) => {
        const conversationId = (res && res.conversationId || "").trim();
        if (!conversationId) return;
        const spec = specs[k] || {};
        items.push({
          type: "subagent",
          id: `item-subagent-${conversationId}`,
          index: i,
          sub: { conversationId, typeName: spec.typeName || "", role: spec.role || "", prompt: spec.initialPrompt || "" },
          step: s
        });
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
    } else if (type.startsWith("CORTEX_STEP_TYPE_") && type !== "CORTEX_STEP_TYPE_SYSTEM_MESSAGE") {
      const rawName = type.replace("CORTEX_STEP_TYPE_", "").toLowerCase();
      let displayName = rawName;
      let detail = "";

      if (s.codeAction) {
        const ca = s.codeAction;
        if (ca.actionSpec?.createFile) {
          displayName = "create_file";
          detail = (ca.actionSpec.createFile.path?.absoluteURI || "").split("/").pop();
        } else if (ca.actionSpec?.editFile) {
          displayName = "edit_file";
          detail = (ca.actionSpec.editFile.path?.absoluteURI || "").split("/").pop();
        } else if (ca.actionSpec?.deleteFile) {
          displayName = "delete_file";
          detail = (ca.actionSpec.deleteFile.path?.absoluteURI || "").split("/").pop();
        }
      } else if (s.toolCall) {
        displayName = s.toolCall.name || rawName;
        if (s.toolCall.toolSummary) {
          detail = s.toolCall.toolSummary;
        }
      } else if (rawName === "shell_command") {
        displayName = "command";
        if (s.shellCommand?.commandLine) {
          detail = s.shellCommand.commandLine.slice(0, 40);
        }
      }

      if (!currentBatch) {
        currentBatch = {
          type: "tools",
          id: `item-tools-${i}`,
          startIndex: i,
          steps: [],
          toolNames: []
        };
      }

      currentBatch.steps.push({
        name: displayName,
        detail: detail,
        status: s.status || "DONE",
        raw: s
      });

      if (!currentBatch.toolNames.includes(displayName) && currentBatch.toolNames.length < 3) {
        currentBatch.toolNames.push(displayName);
      }
    }
  }

  flushBatch();
  return items;
}

function getItemFingerprint(item, isRunning, isLastItem) {
  if (!item) return "";
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
    return `t:${item.steps.length}:${item.toolNames.join(",")}:${active}`;
  }
  if (item.type === "error") {
    return `e:${item.text.length}:${item.text.slice(-12)}`;
  }
  if (item.type === "subagent") {
    const live = SubagentManager.byId.get(item.sub.conversationId);
    return `s:${item.sub.conversationId}:${live ? live.status : ""}:${live ? live.stepCount || 0 : 0}`;
  }
  return "";
}

function formatToolName(name) {
  if (!name) return "工具操作";
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
  if (map[lower]) return map[lower];
  if (lower.startsWith("mcp_")) return `MCP: ${raw.slice(4)}`;
  return raw;
}

function generateItemHtml(item, isRunning, isLastItem) {
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
            <button type="button" class="rescue-btn primary" onclick="sendCompactCommand()">一键压缩会话 (/compact)</button>
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
    const renderedUserText = item.text
      ? (typeof renderUserTextWithSlashPills === "function"
          ? renderUserTextWithSlashPills(item.text, item.slashCommands)
          : escapeHtml(item.text))
      : "";
    const textHtml = renderedUserText ? `<div>${renderedUserText}</div>` : "";
    return `<div class="bubble-col"><div class="bubble">${imagesHtml}${textHtml}</div>${buildMessageActionsHtml("user")}</div>`;
  }

  if (item.type === "agent") {
    let thoughtHtml = "";
    if (item.thinking) {
      thoughtHtml = `
        <details class="thought-box">
          <summary>🧠 Agent 思考过程 (${item.thinking.length} 字符)</summary>
          <div class="thought-content">${escapeHtml(item.thinking)}</div>
        </details>
      `;
    }
    const bodyHtml = item.text ? getCachedMarkdown(item.text) : '<span style="color:var(--text-muted);">执行中...</span>';
    let artifactsHtml = "";
    if (Array.isArray(item.artifacts) && item.artifacts.length > 0) {
      artifactsHtml = `<div class="message-artifact-cards">` +
        item.artifacts.map(art => {
          const title = escapeHtml(art.title || "文档详情");
          const summary = art.summary ? `<div class="artifact-card-summary">${escapeHtml(art.summary)}</div>` : "";
          const uriAttr = escapeHtml(art.uri || "");
          return `
            <div class="artifact-preview-card" data-uri="${uriAttr}" data-title="${title}">
              <div class="artifact-card-header">
                <span class="artifact-card-icon">📄</span>
                <span class="artifact-card-title">${title}</span>
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
          <div class="agent-message-body">${bodyHtml}</div>
          ${artifactsHtml}
        </div>
        ${buildMessageActionsHtml("agent")}
      </div>
    `;
  }

  if (item.type === "tools") {
    const isActive = isRunning && isLastItem;
    const count = item.steps.length;
    const localizedNames = [...new Set((item.toolNames || []).map(formatToolName))];
    const toolNamesStr = localizedNames.join(", ") + (localizedNames.length > 2 ? "..." : "");

    if (isActive) {
      return `
        <div class="agent-thinking-card">
          <div class="agent-avatar">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
              <path d="M12 2v4m0 12v4M4.93 4.93l2.83 2.83m8.48 8.48l2.83 2.83M2 12h4m12 0h4M4.93 19.07l2.83-2.83m8.48-8.48l2.83-2.83"></path>
            </svg>
          </div>
          <div class="agent-thinking-body">
            <div class="thinking-title-row">
              <span>Agent 正在思考与执行</span>
              <div class="activity-dots">
                <span class="dot"></span>
                <span class="dot"></span>
                <span class="dot"></span>
              </div>
            </div>
            <div class="active-tools-pill">
              <svg class="bolt-icon" width="10" height="10" viewBox="0 0 24 24" fill="currentColor">
                <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"></polygon>
              </svg>
              <span>已执行 <strong>${count}</strong> 项操作</span>
              ${toolNamesStr ? `<span class="tool-names">(${escapeHtml(toolNamesStr)})</span>` : ''}
            </div>
          </div>
        </div>
      `;
    }

    const stepItemsHtml = item.steps.map(s => `
      <div class="tool-step-item">
        <div class="tool-step-left">
          <svg class="tool-puzzle-icon" width="13" height="13" viewBox="0 0 24 24" fill="currentColor">
            <path d="M20.5 11H19V7c0-1.1-.9-2-2-2h-4V3.5a2.5 2.5 0 0 0-5 0V5H4c-1.1 0-1.99.9-1.99 2v3.8H3.5c1.49 0 2.7 1.21 2.7 2.7s-1.21 2.7-2.7 2.7H2V20c0 1.1.9 2 2 2h3.8v-1.5c0-1.49 1.21-2.7 2.7-2.7s2.7 1.21 2.7 2.7V22H17c1.1 0 2-.9 2-2v-4h1.5a2.5 2.5 0 0 0 0-5z"></path>
          </svg>
          <span class="tool-step-name">${escapeHtml(formatToolName(s.name))}</span>
        </div>
      </div>
    `).join("");

    return `
      <details class="tool-batch-accordion">
        <summary class="tool-batch-summary">
          <div class="tool-batch-banner">
            <svg class="bolt-icon" width="11" height="11" viewBox="0 0 24 24" fill="currentColor">
              <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"></polygon>
            </svg>
            <span class="tool-batch-title">已思考并执行 <strong>${count}</strong> 项操作</span>
            ${toolNamesStr ? `<span class="tool-names">(${escapeHtml(toolNamesStr)})</span>` : ''}
            <svg class="chevron-icon" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="6 9 12 15 18 9"></polyline>
            </svg>
          </div>
        </summary>
        <div class="tool-batch-expanded">
          ${stepItemsHtml}
        </div>
      </details>
    `;
  }

  return "";
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

  const isLastError = checkLatestMessageIsError(steps, isRunning);
  updateContinueButton(isLastError);

  const items = groupSteps(steps);
  const isSessionSwitch = streamEl.__currentCascadeId !== activeCascadeId;
  streamEl.__currentCascadeId = activeCascadeId;

  const lastItem = items[items.length - 1];
  const isAwaiting = isRunning && lastItem?.type !== "tools";

  let hasDOMChanges = false;

  if (isSessionSwitch) {
    // P2 / B-5 Fast Path: Session switch! Replace the entire child tree in one single
    // atomic DOM layout operation via DocumentFragment + replaceChildren to eliminate
    // tens of layout reflows and eliminate stuttering.
    const fragment = document.createDocumentFragment();
    for (let i = 0; i < items.length; i++) {
      const item = items[i];
      const isLastItem = (i === items.length - 1);
      const fp = getItemFingerprint(item, isRunning, isLastItem);

      let rowClass = "message-row";
      if (item.type === "user") {
        rowClass = "message-row user";
      } else if (item.type === "agent") {
        rowClass = "message-row agent";
      } else if (item.type === "tools") {
        rowClass = (isRunning && isLastItem) ? "message-row agent" : "message-row tool-batch-row";
      } else if (item.type === "error") {
        rowClass = "message-row agent error-row";
      } else if (item.type === "subagent") {
        rowClass = "message-row subagent-row";
      }

      const newEl = document.createElement("div");
      newEl.id = item.id;
      newEl.className = rowClass;
      newEl.setAttribute("data-fp", fp);
      newEl.innerHTML = generateItemHtml(item, isRunning, isLastItem);
      fragment.appendChild(newEl);
    }

    if (isAwaiting) {
      const thinkingIndicator = document.createElement("div");
      thinkingIndicator.id = "agent-thinking-indicator";
      thinkingIndicator.className = "agent-thinking-card";
      thinkingIndicator.innerHTML = `
        <div class="agent-avatar">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
            <path d="M12 2v4m0 12v4M4.93 4.93l2.83 2.83m8.48 8.48l2.83 2.83M2 12h4m12 0h4M4.93 19.07l2.83-2.83m8.48-8.48l2.83-2.83"></path>
          </svg>
        </div>
        <div class="agent-thinking-body">
          <div class="thinking-title-row">
            <span>Agent 正在思考与执行</span>
            <div class="activity-dots">
              <span class="dot"></span>
              <span class="dot"></span>
              <span class="dot"></span>
            </div>
          </div>
        </div>
      `;
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
      const fp = getItemFingerprint(item, isRunning, isLastItem);

      let rowClass = "message-row";
      if (item.type === "user") {
        rowClass = "message-row user";
      } else if (item.type === "agent") {
        rowClass = "message-row agent";
      } else if (item.type === "tools") {
        rowClass = (isRunning && isLastItem) ? "message-row agent" : "message-row tool-batch-row";
      } else if (item.type === "error") {
        rowClass = "message-row agent error-row";
      } else if (item.type === "subagent") {
        rowClass = "message-row subagent-row";
      }

      let existingEl = document.getElementById(item.id);
      if (existingEl) {
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

            const bodyHtml = item.text ? getCachedMarkdown(item.text) : '<span style="color:var(--text-muted);">执行中...</span>';
            bodyEl.innerHTML = bodyHtml;
          } else {
            const wasOpen = existingEl.querySelector("details")?.open;
            existingEl.innerHTML = generateItemHtml(item, isRunning, isLastItem);
            if (wasOpen) {
              const newDetails = existingEl.querySelector("details");
              if (newDetails) newDetails.open = true;
            }
          }
          hasDOMChanges = true;
        }
      } else {
        const newEl = document.createElement("div");
        newEl.id = item.id;
        newEl.className = rowClass + " message-entering";
        newEl.setAttribute("data-fp", fp);
        newEl.innerHTML = generateItemHtml(item, isRunning, isLastItem);

        const indicator = document.getElementById("agent-thinking-indicator");
        if (indicator) {
          streamEl.insertBefore(newEl, indicator);
        } else {
          streamEl.appendChild(newEl);
        }
        hasDOMChanges = true;
      }
    }

    let thinkingIndicator = document.getElementById("agent-thinking-indicator");
    if (isAwaiting) {
      if (!thinkingIndicator) {
        thinkingIndicator = document.createElement("div");
        thinkingIndicator.id = "agent-thinking-indicator";
        thinkingIndicator.className = "agent-thinking-card";
        thinkingIndicator.innerHTML = `
          <div class="agent-avatar">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
              <path d="M12 2v4m0 12v4M4.93 4.93l2.83 2.83m8.48 8.48l2.83 2.83M2 12h4m12 0h4M4.93 19.07l2.83-2.83m8.48-8.48l2.83-2.83"></path>
            </svg>
          </div>
          <div class="agent-thinking-body">
            <div class="thinking-title-row">
              <span>Agent 正在思考与执行</span>
              <div class="activity-dots">
                <span class="dot"></span>
                <span class="dot"></span>
                <span class="dot"></span>
              </div>
            </div>
          </div>
        `;
        streamEl.appendChild(thinkingIndicator);
        hasDOMChanges = true;
      }
    } else if (thinkingIndicator) {
      thinkingIndicator.remove();
      hasDOMChanges = true;
    }
  }

  // 1. First-time render on entering a conversation: align cleanly to bottom without whole-page jump
  if (!hasInitiallyAligned) {
    hasInitiallyAligned = true;
    prevWasRunning = isRunning;
    streamEl.scrollTop = streamEl.scrollHeight;
    return;
  }

  // 2. Active task finished: preserve position stably without upward jumping
  const justFinished = (prevWasRunning && !isRunning);
  prevWasRunning = isRunning;

  if (justFinished) {
    LocalQueueManager.onAgentCompleted();
    if (typeof refreshActionAvailability === "function") refreshActionAvailability();
    if (userIsNearBottom && !isUserTouching) {
      streamEl.scrollTop = streamEl.scrollHeight;
    }
  }

  // 3. Live streaming while running: pin to bottom ONLY if user is near bottom and not actively dragging
  if (isRunning && hasDOMChanges && userIsNearBottom && !isUserTouching) {
    streamEl.scrollTop = streamEl.scrollHeight;
  }

  // Render any new mermaid diagram blocks in messages stream
  if (hasDOMChanges) {
    renderAllMermaidDiagrams(streamEl);
  }

  setTimeout(() => updatePromptJumpStepper(), 50);
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
    this.isExpanded = (storedExpanded !== null) ? (storedExpanded === "true") : true;
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
    if (!skipConfirm && !confirm("确定要终止此后台任务吗？")) return;

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
      alert("终止任务失败: " + err.message);
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

    listEl.innerHTML = this.tasks.map(task => {
      const desc = escapeHtml(task.toolSummary || task.toolAction || formatToolName(task.toolName) || "后台任务");
      const cmd = escapeHtml(task.commandLine || "运行中...");
      const idEsc = escapeHtml(task.id || "");
      return `
        <div class="task-item-row" data-id="${idEsc}" data-step="${task.stepIndex}">
          <div class="task-item-body">
            <span class="task-item-desc">${desc}</span>
            <span class="task-item-cmd" title="${cmd}">${cmd}</span>
          </div>
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

  enqueue(text) {
    const t = (text || '').trim();
    if (!t) return;
    if (this.deletedTombstones) {
      this.deletedTombstones = this.deletedTombstones.filter(it => (it.text || '').trim() !== t);
    }
    const item = {
      id: `queue-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`,
      text: t,
      createdAt: new Date().toISOString()
    };
    this.queue.push(item);
    this.save();
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
      alert("发送失败: " + err.message);
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
        thumbHtml = `<img class="queued-item-thumb" src="${escapeHtml(item.imageUrls[0])}" alt="attachment" />`;
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

// 语音连接失败后「一键排查」发送给智能体的预设提示词（三端保持一致）
const VOICE_TROUBLESHOOT_PROMPT = [
  "【任务】排查「本机无法使用谷歌语音(Gemini Live)功能」的问题。",
  "",
  "【现象】",
  "- 本机 Antigravity 的语音功能无法连接，界面提示 “Couldn't connect / Try again”",
  "",
  "【请按顺序排查】",
  "1. 登录与授权：",
  "   - 检查登录态的 OAuth Scope 是否完整。旧登录态可能缺少语音所需的新权限",
  "   - 如需重新授权，先退出 Antigravity，将 jetski-standalone-oauth-token 文件重命名或移走，再启动并重新登录，以强制拉取完整 Scope",
  "2. 代理与长连接：",
  "   - 语音使用双向流式通信（bidi stream / gRPC）",
  "   - 确认代理（Clash / Clash Verge / Sing-box 等）已开启 TUN 模式，或支持对 *.googleapis.com 的 HTTP/2 与 gRPC 流式转发",
  "   - 查看日志是否出现 context canceled while waiting for connections（连接超时的典型特征）",
  "3. 系统麦克风：",
  "   - 确认系统麦克风访问权限与「允许桌面应用访问麦克风」均已开启",
  "   - 确认默认录音设备正常，且没有被其他程序占用",
  "4. 网关链路：检查网关语音 WebSocket 链路与日志（mgy 会话输出）",
  "",
  "【输出要求】",
  "- 先给出根因判断与证据（日志、命令输出），再给修复步骤",
  "- 能自动修复的直接修复；涉及改配置、重启服务或重新登录的，先说明再执行",
  "- 修复后验证语音是否恢复"
].join("\n");

// --- Full-Duplex Real-time Voice Call Manager (jetski-voice-proto) ---
const WebVoiceCallManager = {
  lastTroubleshootBubbleAt: 0,
  isInCall: false,
  callState: "idle", // idle | connecting | listening | reconnecting

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

  // 连接明确失败（已自动挂断）后弹出排查气泡；30 秒内不重复弹
  showTroubleshootBubble() {
    const now = Date.now();
    if (now - this.lastTroubleshootBubbleAt < 30000) return;
    this.lastTroubleshootBubbleAt = now;
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
    inputEl.value = VOICE_TROUBLESHOOT_PROMPT;
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
    } else if (stateVal === 4 || stateVal === 5) {
      this.endCall();
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

function updateContextMeterUI(meter, isRunning = false) {
  currentContextMeter = meter;
  const ringBtn = document.getElementById("chat-context-ring-btn");
  const ringCircle = document.getElementById("context-ring-circle");
  const ringText = document.getElementById("context-ring-text");
  const speedBadge = document.getElementById("context-speed-badge");

  if (!ringBtn || !meter) {
    if (ringBtn) ringBtn.classList.add("hidden");
    return;
  }

  ringBtn.classList.remove("hidden");
  const pct = Math.min(Math.max(meter.percentage || 0, 0), 100);
  if (ringCircle) {
    ringCircle.setAttribute("stroke-dasharray", `${pct.toFixed(1)}, 100`);
    ringCircle.classList.remove("warn", "danger");
    if (pct >= 90 || meter.isOverflow) {
      ringCircle.classList.add("danger");
    } else if (pct >= 70) {
      ringCircle.classList.add("warn");
    }
  }
  if (ringText) {
    ringText.textContent = `${Math.round(pct)}%`;
  }
  if (speedBadge) {
    if ((isRunning || meter.tokensPerSecond > 0) && meter.tokensPerSecond > 0) {
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

  const pct = Math.min(Math.max(meter.percentage || 0, 0), 100);
  if (usedEl) usedEl.textContent = (meter.usedTokens || 0).toLocaleString();
  if (maxEl) maxEl.textContent = meter.maxTokens >= 1000 ? `${Math.round(meter.maxTokens / 1000)}k` : String(meter.maxTokens || 200000);
  if (fillEl) {
    fillEl.style.width = `${pct}%`;
    fillEl.style.background = pct >= 90 ? "var(--ios-red, #ff3b30)" : pct >= 70 ? "var(--ios-orange, #ff9500)" : "var(--ios-green, #34c759)";
  }
  if (ttftEl) ttftEl.textContent = meter.timeToFirstTokenSec > 0 ? meter.timeToFirstTokenSec.toFixed(2) : "--";
  if (tpsEl) tpsEl.textContent = meter.tokensPerSecond > 0 ? meter.tokensPerSecond.toFixed(1) : "--";
  if (durEl) durEl.textContent = meter.streamingDurationSec > 0 ? meter.streamingDurationSec.toFixed(2) : "--";

  if (inpEl) inpEl.textContent = (meter.inputTokens || 0).toLocaleString();
  if (capEl) capEl.textContent = (meter.cacheReadTokens || 0).toLocaleString();
  if (outEl) outEl.textContent = (meter.outputTokens || 0).toLocaleString();
  if (thinkEl) thinkEl.textContent = (meter.thinkingTokens || 0).toLocaleString();
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

