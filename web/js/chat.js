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

function renderInteractionCard() {
  const container = document.getElementById("interaction-card-container");
  if (!container || !currentPendingInteraction) return;

  const interaction = currentPendingInteraction;
  const isPermission = interaction.type === "permission";
  const hasMultiQuestions = interaction.questions && interaction.questions.length > 1;

  const iconSvg = isPermission
    ? `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"></path></svg>`
    : `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"></path><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>`;

  let contentHtml = '';
  if (hasMultiQuestions) {
    contentHtml = `
      <div class="interaction-multi-questions-list" style="max-height: 280px; overflow-y: auto; display: flex; flex-direction: column; gap: 12px; margin-bottom: 8px; padding-right: 4px;">
        ${interaction.questions.map((q, qIdx) => {
          const selectedId = multiQuestionSelections[qIdx] || q.defaultOptionId || q.options?.[0]?.id || "1";
          const isDenySelected = selectedId === "5" || selectedId === "__write_in__" || selectedId.toLowerCase() === "other";
          return `
            <div class="interaction-question-block" style="border: 1px solid var(--border-color, rgba(255,255,255,0.08)); border-radius: 8px; padding: 10px; background: rgba(0,0,0,0.15);">
              <div style="display: flex; align-items: flex-start; gap: 6px; margin-bottom: 8px;">
                <span style="background: #9333ea; color: white; font-size: 10px; font-weight: bold; padding: 2px 5px; border-radius: 4px; font-family: monospace;">Q${qIdx + 1}</span>
                <span style="font-size: 13px; font-weight: 600; color: var(--text-primary); line-height: 1.3;">${escapeHtml(q.question)}</span>
              </div>
              <div class="interaction-options-list" style="display: flex; flex-direction: column; gap: 5px;">
                ${(q.options || []).map(opt => {
                  const isSelected = opt.id === selectedId;
                  const label = opt.text || opt.label || "";
                  return `
                    <div class="interaction-option-item ${isSelected ? 'selected' : ''}" data-q-idx="${qIdx}" data-opt-id="${escapeHtml(opt.id)}" style="display: flex; align-items: center; gap: 8px; padding: 6px 9px; border-radius: 6px; cursor: pointer; background: ${isSelected ? 'rgba(59,130,246,0.12)' : 'var(--bg-tertiary, rgba(255,255,255,0.04))'}; border: 1px solid ${isSelected ? '#3b82f6' : 'transparent'};">
                      <span class="interaction-option-radio" style="width: 14px; height: 14px; border-radius: 50%; border: 1.5px solid ${isSelected ? '#3b82f6' : 'var(--text-secondary)'}; display: flex; align-items: center; justify-content: center; flex-shrink: 0;">
                        ${isSelected ? '<span style="width: 6px; height: 6px; border-radius: 50%; background: #3b82f6;"></span>' : ''}
                      </span>
                      <span class="interaction-option-badge" style="font-size: 11px; font-weight: bold; font-family: monospace; color: ${isSelected ? '#3b82f6' : 'var(--text-secondary)'}; flex-shrink: 0;">[${escapeHtml(opt.id)}]</span>
                      <span class="interaction-option-label" style="font-size: 12.5px; color: ${isSelected ? 'var(--text-primary)' : 'var(--text-secondary)'}; overflow-x: auto; white-space: nowrap; flex: 1; scrollbar-width: none;">${escapeHtml(label)}</span>
                    </div>
                  `;
                }).join('')}
              </div>
              ${q.hasWriteIn && isDenySelected ? `
                <div style="margin-top: 6px;">
                  <input type="text" class="interaction-multi-write-in-input" data-q-idx="${qIdx}" placeholder="${escapeHtml(q.writeInPlaceholder || '输入说明...')}" value="${escapeHtml(multiQuestionWriteIns[qIdx] || '')}" style="width: 100%; font-size: 12px; padding: 5px 8px; border-radius: 6px; border: 1px solid var(--border-color); background: var(--bg-primary); color: var(--text-primary);" />
                </div>
              ` : ''}
            </div>
          `;
        }).join('')}
      </div>
    `;
  } else {
    const selectedOpt = (interaction.options || []).find(o => o.id === selectedInteractionOptionId) || interaction.options?.[0];
    const isDenyOrWriteIn = selectedOpt?.isDeny || selectedOpt?.id === "5" || selectedOpt?.id === "no" || selectedOpt?.id?.toLowerCase().includes("deny");

    contentHtml = `
      <div class="interaction-options-list" style="display: flex; flex-direction: column; gap: 6px;">
        ${(interaction.options || []).map((opt, idx) => {
          const isSelected = opt.id === selectedInteractionOptionId;
          const label = opt.text || opt.label || "";
          return `
            <div class="interaction-option-item ${isSelected ? 'selected' : ''}" data-opt-id="${escapeHtml(opt.id)}" style="display: flex; align-items: center; gap: 8px; padding: 8px 10px; border-radius: 8px; cursor: pointer; background: ${isSelected ? 'rgba(59,130,246,0.12)' : 'var(--bg-tertiary, rgba(255,255,255,0.04))'}; border: 1px solid ${isSelected ? '#3b82f6' : 'transparent'};">
              <span class="interaction-option-radio" style="width: 16px; height: 16px; border-radius: 50%; border: 1.5px solid ${isSelected ? '#3b82f6' : 'var(--text-secondary)'}; display: flex; align-items: center; justify-content: center; flex-shrink: 0;">
                ${isSelected ? '<span style="width: 8px; height: 8px; border-radius: 50%; background: #3b82f6;"></span>' : ''}
              </span>
              <span class="interaction-option-badge" style="font-size: 11px; font-weight: bold; font-family: monospace; color: ${isSelected ? '#3b82f6' : 'var(--text-secondary)'}; flex-shrink: 0;">[${escapeHtml(opt.id || String(idx + 1))}]</span>
              <span class="interaction-option-label" style="font-size: 13px; color: ${isSelected ? 'var(--text-primary)' : 'var(--text-secondary)'}; overflow-x: auto; white-space: nowrap; flex: 1; scrollbar-width: none;">${escapeHtml(label)}</span>
            </div>
          `;
        }).join('')}
      </div>

      <div id="interaction-write-in-wrap" class="interaction-write-in-wrap ${isDenyOrWriteIn ? '' : 'hidden'}" style="margin-top: 8px;">
        <input type="text" id="interaction-write-in-input" class="interaction-write-in-input" placeholder="${escapeHtml(interaction.writeInPlaceholder || '输入说明或拒绝原因...')}" style="width: 100%; font-size: 12px; padding: 6px 10px; border-radius: 6px; border: 1px solid var(--border-color); background: var(--bg-primary); color: var(--text-primary);" />
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
        <div style="flex: 1;"></div>
        <button type="button" id="btn-interaction-skip" class="btn-interaction-skip" ${isSubmittingInteraction ? 'disabled' : ''} style="padding: 6px 14px; border-radius: 8px; border: 1px solid var(--border-color); background: transparent; color: var(--text-secondary); cursor: pointer;">Skip</button>
        <button type="button" id="btn-interaction-submit" class="btn-interaction-submit" ${isSubmittingInteraction ? 'disabled' : ''} style="padding: 6px 16px; border-radius: 8px; border: none; background: #2563eb; color: white; font-weight: 600; cursor: pointer;">
          <span>${isSubmittingInteraction ? '提交中...' : 'Submit'}</span>
          <span class="interaction-submit-key">↵</span>
        </button>
      </div>
    </div>
  `;

  container.classList.remove("hidden");

  // Multi-question option click handlers
  container.querySelectorAll(".interaction-option-item[data-q-idx]").forEach(item => {
    item.addEventListener("click", () => {
      const qIdx = parseInt(item.dataset.qIdx, 10);
      const optId = item.dataset.optId;
      if (optId) {
        multiQuestionSelections[qIdx] = optId;
        renderInteractionCard();
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
    item.addEventListener("click", () => {
      const optId = item.dataset.optId;
      if (optId && optId !== selectedInteractionOptionId) {
        selectedInteractionOptionId = optId;
        renderInteractionCard();
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

function initScrollListener() {
  const streamEl = document.getElementById("messages-stream");
  if (!streamEl || streamEl.dataset.hasScrollListener) return;
  streamEl.dataset.hasScrollListener = "true";

  const updateNearBottom = () => {
    const dist = streamEl.scrollHeight - streamEl.scrollTop - streamEl.clientHeight;
    userIsNearBottom = dist <= 80;
  };

  streamEl.addEventListener("scroll", updateNearBottom, { passive: true });
  streamEl.addEventListener("touchstart", () => { isUserTouching = true; }, { passive: true });
  streamEl.addEventListener("touchend", () => {
    isUserTouching = false;
    updateNearBottom();
  }, { passive: true });
  streamEl.addEventListener("touchcancel", () => {
    isUserTouching = false;
    updateNearBottom();
  }, { passive: true });
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
      const wsName = wsUri.split("/").filter(Boolean).pop() || "Chat";
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
    const hasContent = (chatInput && chatInput.value.trim().length > 0) || (pendingImages && pendingImages.length > 0);

    if (isRunning) {
      if (hasContent) {
        sendBtn.className = "btn-action-circle send-mode active";
        sendBtn.title = "加入待发送队列";
        sendBtn.setAttribute("aria-label", "加入待发送队列");
        if (iconSend) iconSend.classList.remove("hidden");
        if (iconStop) iconStop.classList.add("hidden");
      } else {
        sendBtn.className = "btn-action-circle stop-mode";
        sendBtn.title = "停止任务";
        sendBtn.setAttribute("aria-label", "停止任务");
        if (iconSend) iconSend.classList.add("hidden");
        if (iconStop) iconStop.classList.remove("hidden");
      }
    } else {
      sendBtn.className = "btn-action-circle send-mode";
      if (hasContent) sendBtn.classList.add("active");
      sendBtn.title = "发送";
      sendBtn.setAttribute("aria-label", "发送");
      if (iconSend) iconSend.classList.remove("hidden");
      if (iconStop) iconStop.classList.add("hidden");
    }
  }
}

async function connectStreamWs(cascadeId) {
  if (activeWs && activeWs.__cascadeId === cascadeId && (activeWs.readyState === WebSocket.OPEN || activeWs.readyState === WebSocket.CONNECTING)) {
    return;
  }
  closeActiveWs();

  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  let wsUrl = `${proto}//${location.host}/gateway/cascade/stream?cascadeId=${encodeURIComponent(cascadeId)}`;
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

  // Abort if active cascade changed during ticket exchange
  if (activeCascadeId !== cascadeId) return;
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

        if (data.activeModel) {
          syncActiveModel(data.activeModel);
        }

        const isRunning = data.status === "CASCADE_RUN_STATUS_RUNNING" || (data.runningTasks && data.runningTasks.length > 0);

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
          currentTrajectories[cascadeId].stepCount = data.totalSteps;
          currentTrajectories[cascadeId].needsInput = hasAction;
        }

        if (data.queuedMessages !== undefined) {
          LocalQueueManager.syncFromServer(data.queuedMessages, data.steps || data.messages);
        } else if (data.steps) {
          LocalQueueManager.syncFromServer(null, data.steps);
        }

        RunningTasksManager.syncFromServer(data.runningTasks);

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
    pollTimer = setInterval(() => {
      if (activeCascadeId === cascadeId && document.visibilityState === "visible") {
        loadChat(cascadeId, true);
      }
    }, 1500);
  }
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
    const isRunning = summary?.status === "CASCADE_RUN_STATUS_RUNNING" || (summary?.runningTasks && summary.runningTasks.length > 0);
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
            currentCanProceed = !!info.canProceed && !isRunning;
            currentProceedArtifactUri = info.proceedArtifactUri || null;
            updateProceedButton(currentCanProceed);
            updatePendingInteraction(info.pendingInteraction || null, isRunning);
            const hasAction = !!(info.pendingInteraction || currentCanProceed);
            updateChatControls(isRunning, wsUri, hasAction);
            if (currentTrajectories[cascadeId]) {
              currentTrajectories[cascadeId].needsInput = hasAction;
            }
          }
        })
        .catch(() => {});
    }

    if (isRunning && (!activeWs || activeWs.readyState !== WebSocket.OPEN) && !pollTimer) {
      pollTimer = setInterval(() => {
        if (activeCascadeId === cascadeId && document.visibilityState === "visible") {
          loadChat(cascadeId, true);
        }
      }, 1500);
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
      const userText = (s.userInput?.userResponse || s.userInput?.items?.[0]?.text || "").trim();
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
          media: mediaList,
          imageUrls: imageUrls,
          step: s
        });
      }
    } else if (type === "CORTEX_STEP_TYPE_PLANNER_RESPONSE") {
      const p = s.plannerResponse || {};
      const resp = (p.response || "").trim();
      const thinking = (p.thinking || "").trim();

      if (resp) {
        flushBatch();
        const artList = s.artifacts ? [...s.artifacts] : [];
        items.push({
          type: "agent",
          id: `item-agent-${i}`,
          index: i,
          text: p.response,
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
      items.push({
        type: "error",
        id: `item-error-${i}`,
        index: i,
        text: errText,
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
    return `u:${item.text.length}:${item.text.slice(-10)}:m${mediaLen}`;
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
          return `<img src="${src}" class="bubble-image" data-action="open-image" alt="上传图片" />`;
        }).join("") + `</div>`;
    }
    const textHtml = item.text ? `<div>${escapeHtml(item.text)}</div>` : "";
    return `<div class="bubble">${imagesHtml}${textHtml}</div>`;
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
      <div class="bubble markdown-body">
        ${thoughtHtml}
        <div class="agent-message-body">${bodyHtml}</div>
        ${artifactsHtml}
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
}

// --- Running Background Tasks Manager (Desktop Antigravity Parity) ---
const RunningTasksManager = {
  tasks: [],
  isExpanded: true,

  init(cascadeId) {
    if (!cascadeId) return;
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
    if (!cascadeId) return;
    this.deletedTombstones = [];
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
      const isRunning = currentTrajectories[activeCascadeId]?.status === "CASCADE_RUN_STATUS_RUNNING";
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
        thumbHtml = `<img class="queued-item-thumb" src="${src}" alt="attachment" />`;
      } else if (item.imageUrls && item.imageUrls.length > 0) {
        thumbHtml = `<img class="queued-item-thumb" src="${escapeHtml(item.imageUrls[0])}" alt="attachment" />`;
      }
      return `
      <div class="queued-item-row" data-id="${item.id}">
        ${thumbHtml}
        <span class="queued-item-text">${escapeHtml(item.text || (thumbHtml ? "图片" : ""))}</span>
        <div class="queued-actions" data-testid="queued-decorators">
          <button class="queued-icon-btn btn-send-now" data-action="queue-send" data-id="${item.id}" title="立即发送" aria-label="立即发送">
            <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 -960 960 960" fill="currentColor">
              <path d="M665.08-450H180v-60H665.08L437.23-737.85L480-780L780-480L480-180l-42.77-42.15L665.08-450Z"></path>
            </svg>
          </button>
          <button class="queued-icon-btn btn-edit" data-action="queue-edit" data-id="${item.id}" title="编辑" aria-label="编辑">
            <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 -960 960 960" fill="currentColor">
              <path d="M200-200h50.46L659.92-609.46l-50.46-50.46L200-250.46V-200Zm-60,60V-275.38L667.62-802.77q9.07-8.24 20.04-12.74T710.65-820t23.31,4.27t19.97,13.58l48.85,49.46q9.31,8.69 13.27,20T820-710.07q0,12.07-4.12,23.03T802.77-667L275.38-140H140ZM760.38-710.15l-50.23-50.23l50.23,50.23Zm-126.13,75.9l-24.79-25.67l50.46,50.46l-25.67-24.79Z"></path>
            </svg>
          </button>
          <button class="queued-icon-btn btn-delete" data-action="queue-remove" data-id="${item.id}" title="删除" aria-label="删除">
            <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 -960 960 960" fill="currentColor">
              <path d="M292.31-140q-29.92,0-51.11-21.19T220-212.31V-720H180v-60H360v-35.38H600V-780H780v60H740v507.69Q740-182 719-161t-51.31,21H292.31ZM680-720H280v507.69q0,5.39 3.46,8.85t8.85,3.46H667.69q4.62,0 8.46-3.85t3.85-8.46V-720ZM376.16-280h60V-640h-60v360Zm147.69,0h60V-640h-60v360ZM280-720v507.69q0,5.39 0,8.85t0,3.46q0,0 0-3.46t0-8.85V-720Z"></path>
            </svg>
          </button>
        </div>
      </div>
    `;}).join("");
  }
};
