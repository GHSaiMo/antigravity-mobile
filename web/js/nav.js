// --- Navigation & Routing ---

function formatConversationTitle(annotations, summary, fallback = "未命名会话") {
  const defaultFallback = typeof t === "function" ? t(fallback) : fallback;
  const raw = annotations?.title || summary;
  if (!raw || raw === "未命名会话" || raw === "Untitled Conversation") return defaultFallback;
  const cleaned = raw.replace(/<[^>]+>/g, "").trim();
  const firstLine = cleaned.split("\n").map((l) => l.trim()).find((l) => l.length > 0) || "";
  if (!firstLine) return defaultFallback;
  if (firstLine.length > 36) {
    return firstLine.slice(0, 36) + "...";
  }
  return firstLine;
}

function renderScheduledAlarmIconHtml() {
  const label = typeof t === "function" ? t("定时任务") : "定时任务";
  const labelEsc = escapeHtml(label);
  return `<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="ws-schedule-icon" title="${labelEsc}" aria-label="${labelEsc}"><circle cx="12" cy="13" r="8"></circle><path d="M12 9v4l2 2"></path><path d="M5 3 2 6"></path><path d="m22 6-3-3"></path><path d="M6.38 18.7 4 21"></path><path d="M17.64 18.67 20 21"></path></svg>`;
}
window.renderScheduledAlarmIconHtml = renderScheduledAlarmIconHtml;

function renderSideQuestionIconHtml(size = 12, extraClass = "") {
  const cls = extraClass ? ` class="${extraClass}"` : "";
  return `<svg${cls} width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M21 16a2 2 0 0 1-2 2H7.5L3 21.5V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"></path><path d="M9.75 8.2a2.3 2.3 0 0 1 4.45.8c0 1.4-2.2 1.7-2.2 2.75"></path><circle cx="12" cy="15" r="1.05" fill="currentColor" stroke="none"></circle></svg>`;
}
window.renderSideQuestionIconHtml = renderSideQuestionIconHtml;

function navigateTo(hash) {
  window.location.hash = hash;
  renderRoute();
}
window.selectCascadeConversation = function (cascadeId) {
  if (cascadeId) {
    navigateTo("#c=" + cascadeId);
  }
};

function renderRoute() {
  const hash = window.location.hash || "#";
  const convView = document.getElementById("view-conversations");
  const chatView = document.getElementById("view-chat");
  const settingsBtn = document.getElementById("btn-settings");
  const newBtn = document.getElementById("btn-new");
  const backBtn = document.getElementById("btn-back");
  const brandHeader = document.getElementById("nav-brand-header");
  const inlineTitle = document.getElementById("nav-inline-title");
  const titleText = document.getElementById("chat-title-text");
  const wsText = document.getElementById("chat-workspace-text");

  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
  stopListEvents();

  // Persist current input text to memory draft & flush deferred steps cache before leaving the session
  const chatInput = document.getElementById("chat-input");
  if (activeCascadeId) {
    if (chatInput) DraftManager.set(activeCascadeId, chatInput.value);
    if (typeof PersistentStepsCache !== "undefined") PersistentStepsCache.flush();
  }

  if (hash.startsWith("#c=")) {
    const newCascadeId = hash.slice(3);
    const changed = activeCascadeId !== newCascadeId;
    if (changed && typeof WebVoiceCallManager !== "undefined" && WebVoiceCallManager.isInCall) {
      WebVoiceCallManager.endCall();
    }
    activeCascadeId = newCascadeId;
    if (changed) {
      SubagentManager.reset(); // 切换会话时清掉上一个会话的子代理 / 只读状态
      hasUserManuallySelectedModel = false;
    }
    markConversationAsRead(newCascadeId);

    // View toggling with iOS NavigationStack slide
    convView.classList.add("pushed-left");
    chatView.classList.add("active");

    // Nav Bar configuration for Chat View
    if (settingsBtn) settingsBtn.classList.add("hidden");
    if (newBtn) newBtn.classList.add("hidden");
    document.getElementById("btn-chat-tools")?.classList.remove("hidden");
    if (backBtn) backBtn.classList.remove("hidden");
    if (brandHeader) brandHeader.classList.add("hidden");
    if (inlineTitle) inlineTitle.classList.remove("hidden");
    if (changed && window.Phase3) {
      Phase3.contextItems = [];
      Phase3.suggestedResponses = [];
      Phase3.suggestedResponseSource = "";
      Phase3.dismissedPredictedSuggestion = null;
      Phase3.renderContextChips?.();
      Phase3.renderSuggestedResponses?.();
    }

    // Title resolution
    const summary = currentTrajectories[activeCascadeId];
    const title = formatConversationTitle(summary?.annotations, summary?.summary, typeof t === "function" ? t("会话详情") : "会话详情");
    const wsUri = summary?.workspaceUris?.[0] || summary?.workspaces?.[0]?.workspaceFolderAbsoluteUri || "";
    const wsName = summary?.workspaceName || wsUri.split("/").filter(Boolean).pop() || "";

    if (titleText) {
      const hasRealTitle = Boolean(summary && (summary.annotations?.title || summary.summary));
      if (hasRealTitle) {
        titleText.setAttribute("data-i18n-skip", "1");
      } else {
        titleText.removeAttribute("data-i18n-skip");
      }
      titleText.textContent = title;
    }
    if (wsText) {
      if (wsName) {
        wsText.title = wsUri || wsName;
        if (summary?.isScheduledTask && wsName !== "Chat") {
          wsText.innerHTML = `📁 ${escapeHtml(wsName)}${renderScheduledAlarmIconHtml()}`;
        } else {
          wsText.textContent = `📁 ${wsName}`;
        }
      } else {
        wsText.title = "";
        wsText.textContent = "";
      }
    }

    if (changed) {
      document.getElementById("btn-commit-push")?.classList.add("hidden");
      document.getElementById("btn-changes")?.classList.add("hidden");
      if (typeof refreshActionAvailability === "function") refreshActionAvailability();
      hasInitiallyAligned = false;
      prevWasRunning = false;
      updatePendingInteraction(null, false);

      // Restore session draft into chat input
      if (chatInput) {
        chatInput.value = DraftManager.get(newCascadeId);
        chatInput.style.height = "auto";
        chatInput.style.height = Math.min(chatInput.scrollHeight, 120) + "px";
        const isRunning = isCascadeRunning(activeCascadeId);
        updateChatControls(isRunning, null, false);
      }

      // Per-session queue / background-task cards (previously only re-initialised by loadChat).
      LocalQueueManager.init(activeCascadeId);
      RunningTasksManager.init(activeCascadeId);

      const streamEl = document.getElementById("messages-stream");
      const cached = sessionStepsCache[activeCascadeId];
      if (cached && cached.steps && cached.steps.length > 0) {
        renderMessages(cached.steps, cached.isRunning);
      } else {
        // P1 / B-3: Query IndexedDB persistent cache first to eliminate loading spinner on cold visits
        PersistentStepsCache.get(activeCascadeId).then(persisted => {
          if (persisted && persisted.steps && persisted.steps.length > 0 && activeCascadeId === newCascadeId) {
            setSessionStepsCache(newCascadeId, persisted);
            renderMessages(persisted.steps, persisted.isRunning);
          } else if (streamEl && activeCascadeId === newCascadeId && (!streamEl.children.length || streamEl.querySelector(".loading-state"))) {
            streamEl.innerHTML = `
              <div class="loading-state">
                <div class="ios-spinner"></div>
                <p>正在同步会话历史与步骤...</p>
              </div>
            `;
          }
        });
      }
    }

    // The stream's "init" frame carries the full step list, so it is the only initial load; a raw
    // GetCascadeTrajectory here would download the whole history a second time. If the stream
    // cannot connect, fallbackToHttpPolling loads it over HTTP instead.
    connectStreamWs(activeCascadeId);
  } else if (hash === "#draft") {
    if (typeof WebVoiceCallManager !== "undefined" && WebVoiceCallManager.isInCall) {
      WebVoiceCallManager.endCall();
    }
    if (!activeDraftSession) {
      activeDraftSession = { isPure: true, name: "新对话", path: "", uri: "", rawId: "outside-of-project" };
    }
    activeCascadeId = null;
    closeActiveWs(true);
    SubagentManager.reset();
    RunningTasksManager.init(null);
    LocalQueueManager.init(null);
    currentCanProceed = false;
    currentProceedArtifactUri = null;
    updateProceedButton(false);
    updateContinueButton(false);
    updatePendingInteraction(null, false);
    document.getElementById("btn-commit-push")?.classList.add("hidden");
    document.getElementById("btn-changes")?.classList.add("hidden");
    if (typeof clearSelectedSlash === "function") clearSelectedSlash();
    if (
      Array.isArray(activeDraftSession.prefillSlashCommands) &&
      activeDraftSession.prefillSlashCommands.length > 0 &&
      typeof slashState !== "undefined"
    ) {
      slashState.selected = activeDraftSession.prefillSlashCommands.map((cmd) => ({
        name: cmd.name,
        title: cmd.title || cmd.name,
        kind: cmd.kind || "skill",
        icon: typeof resolveSlashCommandIcon === "function" ? resolveSlashCommandIcon(cmd.name, cmd.icon, cmd.kind) : (cmd.icon || "schedule"),
        definitionPath: cmd.definitionPath || "",
      }));
      slashState.cascadeId = "__draft__";
      delete activeDraftSession.prefillSlashCommands;
      if (typeof renderSlashChip === "function") renderSlashChip();
    }

    convView.classList.add("pushed-left");
    chatView.classList.add("active");

    if (settingsBtn) settingsBtn.classList.add("hidden");
    if (newBtn) newBtn.classList.add("hidden");
    document.getElementById("btn-chat-tools")?.classList.remove("hidden");
    if (backBtn) backBtn.classList.remove("hidden");
    if (brandHeader) brandHeader.classList.add("hidden");
    if (inlineTitle) inlineTitle.classList.remove("hidden");
    if (window.Phase3) {
      Phase3.contextItems = [];
      Phase3.suggestedResponses = [];
      Phase3.suggestedResponseSource = "";
      Phase3.dismissedPredictedSuggestion = null;
      Phase3.renderContextChips?.();
      Phase3.renderSuggestedResponses?.();
    }
    window.V233?.checkWorkspaceTrust?.("", activeDraftSession?.isPure ? "" : (activeDraftSession?.uri || ""));

    if (titleText) titleText.textContent = activeDraftSession.isPure ? "新对话" : activeDraftSession.name;
    if (wsText) wsText.textContent = activeDraftSession.isPure ? "Chat" : `📁 ${activeDraftSession.name}`;

    const streamEl = document.getElementById("messages-stream");
    if (streamEl) {
      streamEl.__currentCascadeId = null;
      if (typeof window.renderNewChatLaunchpad === "function") {
        window.renderNewChatLaunchpad(streamEl, activeDraftSession);
      } else {
        streamEl.innerHTML = `
          <div class="chat-empty-state">
            <div class="chat-empty-icon">
              <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
                <path d="m12 3-1.912 5.813a2 2 0 0 1-1.275 1.275L3 12l5.813 1.912a2 2 0 0 1 1.275 1.275L12 21l1.912-5.813a2 2 0 0 1 1.275-1.275L21 12l-5.813-1.912a2 2 0 0 1-1.275-1.275L12 3Z"></path>
              </svg>
            </div>
            <div class="chat-empty-title">${activeDraftSession.isPure ? "新对话" : escapeHtml(activeDraftSession.name)}</div>
            <div class="chat-empty-desc">${activeDraftSession.isPure ? "新对话模式，在下方输入指令开启对话" : "已连接工作区，在下方输入指令开启对话"}</div>
          </div>
        `;
      }
    }

    if (chatInput) {
      const prefillText = typeof activeDraftSession.prefillText === "string" ? activeDraftSession.prefillText : "";
      if (prefillText) delete activeDraftSession.prefillText;
      chatInput.value = prefillText;
      chatInput.style.height = "auto";
      if (prefillText) {
        chatInput.style.height = Math.min(chatInput.scrollHeight, 120) + "px";
      }
      // 新建空草稿会话默认自动聚焦输入框，方便用户 0 额外点击直接开始任务输入
      setTimeout(() => {
        if (!activeDraftSession || activeCascadeId) return;
        if (document.activeElement !== chatInput) {
          try { chatInput.focus(); } catch (_) {}
        }
        if (prefillText && typeof chatInput.setSelectionRange === "function") {
          const len = chatInput.value.length;
          chatInput.setSelectionRange(len, len);
        }
      }, 60);
    }
    updateChatControls(false, activeDraftSession.isPure ? "" : activeDraftSession.uri, false);
  } else {
    if (typeof WebVoiceCallManager !== "undefined" && WebVoiceCallManager.isInCall) {
      WebVoiceCallManager.endCall();
    }
    activeCascadeId = null;
    activeDraftSession = null;
    closeActiveWs(true);
    SubagentManager.reset();
    RunningTasksManager.init(null);
    LocalQueueManager.init(null);
    currentCanProceed = false;
    currentProceedArtifactUri = null;
    updateProceedButton(false);
    updateContinueButton(false);
    updatePendingInteraction(null, false);
    document.getElementById("btn-commit-push")?.classList.add("hidden");
    document.getElementById("btn-changes")?.classList.add("hidden");
    if (typeof clearSelectedSlash === "function") clearSelectedSlash();
    if (chatInput) {
      chatInput.value = "";
      chatInput.style.height = "auto";
    }

    // View toggling with iOS NavigationStack pop
    chatView.classList.remove("active");
    convView.classList.remove("pushed-left");
    clearAppBadge();

    // Nav Bar configuration for List View
    if (settingsBtn) settingsBtn.classList.remove("hidden");
    if (newBtn) newBtn.classList.remove("hidden");
    document.getElementById("btn-chat-tools")?.classList.add("hidden");
    if (backBtn) backBtn.classList.add("hidden");
    if (brandHeader) brandHeader.classList.remove("hidden");
    if (inlineTitle) inlineTitle.classList.add("hidden");
    if (window.Phase3) {
      Phase3.contextItems = [];
      Phase3.suggestedResponses = [];
      Phase3.suggestedResponseSource = "";
      Phase3.dismissedPredictedSuggestion = null;
      Phase3.renderContextChips?.();
      Phase3.renderSuggestedResponses?.();
    }
    window.V233?.checkWorkspaceTrust?.("", "");

    loadConversations();
    startListEvents();
  }
}

// --- Conversations List ---

// --- Conversation list live updates ---
// The gateway pushes a bare "changed" hint over /gateway/events; the list itself is still fetched
// through loadConversations(), so filtering/enrichment stay in one place. Connected only while the
// list is on screen and the tab is visible.

let listEventsWs = null;
let listEventsWanted = false;
let listEventsReconnectTimer = null;
let listEventsRefreshTimer = null;
let listEventsAttempts = 0;
let listEventsHadConnection = false;
let listEventsLastRev = null;
let listEventsSeq = 0;

// ---- Connection heartbeat (gateway "beat" frames every 5s) -> dot in the home quota bar ----
// 2 missed beats = attention, 6 missed = disconnected; old gateways never send "beat" -> plain connected.
const LINK_ATTENTION_MS = 12000;
const LINK_DISCONNECTED_MS = 30000;
const LINK_CONNECTING_GRACE_MS = 10000;
const linkHealth = {
  started: false, connected: false, hadConnection: false, supportsBeat: false,
  lastBeatAt: 0, startedAt: 0, desktop: null,
};

function evaluateLinkHealth(now = Date.now()) {
  const s = linkHealth;
  if (!s.started) return "unknown";
  if (!s.connected) {
    if (now - s.startedAt < LINK_CONNECTING_GRACE_MS) return s.hadConnection ? "attention" : "unknown";
    return "disconnected";
  }
  if (!s.supportsBeat) return "connected";
  if (!s.lastBeatAt) return "attention";
  const age = now - s.lastBeatAt;
  if (age >= LINK_DISCONNECTED_MS) return "disconnected";
  if (age >= LINK_ATTENTION_MS) return "attention";
  if (s.desktop === "down") return "attention";
  return "connected";
}

const LINK_LABELS = {
  unknown: "正在连接电脑", connected: "已连接电脑", attention: "连接异常", disconnected: "已断开",
};
const LINK_TITLES = {
  unknown: "连接中", connected: "已连接电脑", attention: "连接不稳或电脑端未就绪", disconnected: "未连接",
};

function renderLinkHealth() {
  const state = evaluateLinkHealth();
  const cls = `link-${state}`;
  const el = document.getElementById("link-health-dot");
  if (el) {
    el.classList.remove("link-unknown", "link-connected", "link-attention", "link-disconnected");
    el.classList.add(cls);
    el.setAttribute("aria-label", LINK_LABELS[state]);
  }
}

let linkPulsePhase = false;
function pulseLinkDot() {
  const el = document.getElementById("link-health-dot");
  if (!el) return;
  linkPulsePhase = !linkPulsePhase;
  el.classList.remove("link-pulse", "link-pulse-a", "link-pulse-b");
  el.classList.add(linkPulsePhase ? "link-pulse-a" : "link-pulse-b");
}

function reconnectListEvents() {
  if (!listEventsWanted) return;
  closeListEventsSocket();
  linkHealth.connected = false;
  linkHealth.startedAt = Date.now();
  renderLinkHealth();
  connectListEvents();
}

setInterval(() => {
  if (document.visibilityState !== "visible") return;
  const settingsOpen = !document.getElementById("sheet-settings")?.classList.contains("hidden");
  if (isConversationListActive() || settingsOpen) {
    renderLinkHealth();
  }
}, 1000);

function isConversationListActive() {
  return !activeCascadeId && !activeDraftSession;
}

function scheduleListRefresh() {
  if (listEventsRefreshTimer) return;
  listEventsRefreshTimer = setTimeout(() => {
    listEventsRefreshTimer = null;
    if (listEventsWanted && isConversationListActive()) loadConversations(true);
  }, 150);
}

function closeListEventsSocket() {
  listEventsSeq++;
  if (listEventsReconnectTimer) {
    clearTimeout(listEventsReconnectTimer);
    listEventsReconnectTimer = null;
  }
  if (listEventsWs) {
    listEventsWs.onopen = listEventsWs.onmessage = listEventsWs.onerror = listEventsWs.onclose = null;
    try { listEventsWs.close(); } catch (_) {}
    listEventsWs = null;
  }
}

function stopListEvents() {
  listEventsWanted = false;
  linkHealth.started = false;
  linkHealth.connected = false;
  renderLinkHealth();
  listEventsAttempts = 0;
  if (listEventsRefreshTimer) {
    clearTimeout(listEventsRefreshTimer);
    listEventsRefreshTimer = null;
  }
  closeListEventsSocket();
}

function startListEvents() {
  listEventsWanted = true;
  if (!linkHealth.started) {
    linkHealth.started = true;
    linkHealth.startedAt = Date.now();
  }
  connectListEvents();
}

async function connectListEvents() {
  if (!listEventsWanted || document.visibilityState !== "visible") return;
  if (listEventsWs && (listEventsWs.readyState === WebSocket.OPEN || listEventsWs.readyState === WebSocket.CONNECTING)) return;
  closeListEventsSocket();
  const mySeq = listEventsSeq;

  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  let wsUrl = `${proto}//${location.host}/gateway/events`;
  if (listEventsLastRev !== null) {
    wsUrl += `?sinceRev=${encodeURIComponent(String(listEventsLastRev))}`;
  }
  try {
    // Same one-time ticket exchange as the cascade stream: no long-lived token in the URL.
    const resp = await originalFetch("/api/v1/auth/ws-ticket", { method: "POST" });
    if (resp.ok) {
      const data = await resp.json();
      if (data && data.ticket) {
        wsUrl += `${wsUrl.includes("?") ? "&" : "?"}ticket=${encodeURIComponent(data.ticket)}`;
      }
    }
  } catch (_) {}
  // The route may have changed or another connectListEvents superseded us while the ticket was in flight.
  if (mySeq !== listEventsSeq || !listEventsWanted || document.visibilityState !== "visible") return;
  if (listEventsWs && (listEventsWs.readyState === WebSocket.OPEN || listEventsWs.readyState === WebSocket.CONNECTING)) return;

  try {
    const ws = new WebSocket(wsUrl);
    listEventsWs = ws;
    ws.onopen = () => {
      listEventsAttempts = 0;
      linkHealth.connected = true;
      linkHealth.hadConnection = true;
      renderLinkHealth();
    };
    ws.onmessage = (event) => {
      let frame;
      try { frame = JSON.parse(event.data); } catch (_) { return; }
      if (frame.type === "hello" || frame.type === "beat" || frame.type === "changed") {
        linkHealth.lastBeatAt = Date.now();
        if (typeof frame.desktop === "string") {
          linkHealth.supportsBeat = true;
          linkHealth.desktop = frame.desktop;
        }
        renderLinkHealth();
        pulseLinkDot();
      }
      if (typeof frame.rev === "number") {
        listEventsLastRev = frame.rev;
      }
      if (frame.type === "changed") {
        scheduleListRefresh();
      } else if (frame.type === "hello") {
        // Skip full HTTP list refetch when gateway confirms revision did not change while disconnected.
        if (listEventsHadConnection && !frame.unchanged) scheduleListRefresh();
        listEventsHadConnection = true;
      }
    };
    ws.onclose = () => {
      if (listEventsWs !== ws) return;
      listEventsWs = null;
      linkHealth.connected = false;
      renderLinkHealth();
      if (!listEventsWanted) return;
      const delay = Math.min(1000 * Math.pow(1.5, listEventsAttempts), 30000);
      listEventsAttempts++;
      listEventsReconnectTimer = setTimeout(connectListEvents, delay);
    };
  } catch (_) {}
}

document.addEventListener("visibilitychange", () => {
  if (document.visibilityState === "visible") {
    if (listEventsWanted) {
      // Don't trust an old green dot after the tab was suspended: restart the connect window.
      linkHealth.connected = false;
      linkHealth.startedAt = Date.now();
      renderLinkHealth();
      connectListEvents();
      if (isConversationListActive()) scheduleListRefresh();
    }
  } else {
    closeListEventsSocket();
  }
});

const REORDER_COOLDOWN_MS = 8000;
let lastConvReorderAt = 0;
let lastConvOrderIds = [];
let lastConvActionIds = new Set();
let lastConvRunningIds = new Set();
let pendingConvReorderTimer = null;

function isConversationNeedsAction(item) {
  if (!item) return false;
  if (item.needsInput) return true;
  const st = String(item.status || "").toUpperCase();
  return st.includes("WAITING");
}

function isConversationRunning(item) {
  if (!item) return false;
  if (item.hasError || item.status === "CASCADE_RUN_STATUS_ERROR") return false;
  if (isConversationNeedsAction(item)) return false;
  return item.status === "CASCADE_RUN_STATUS_RUNNING" || Boolean(item.hasRunningSubagents) || Number(item.swarmWorkingCount || 0) > 0;
}

function applyConversationReorderBuffer(sortedItems, forceReorder = false) {
  const now = Date.now();
  const currentActionIds = new Set(sortedItems.filter(isConversationNeedsAction).map((item) => item.id));
  const currentRunningIds = new Set(sortedItems.filter(isConversationRunning).map((item) => item.id));
  let hasNewPendingAction = false;
  for (const id of currentActionIds) {
    if (!lastConvActionIds.has(id)) {
      hasNewPendingAction = true;
      break;
    }
  }
  let hasNewRunning = false;
  for (const id of currentRunningIds) {
    if (!lastConvRunningIds.has(id)) {
      hasNewRunning = true;
      break;
    }
  }
  if (
    forceReorder ||
    hasNewPendingAction ||
    hasNewRunning ||
    lastConvOrderIds.length === 0 ||
    lastConvReorderAt === 0 ||
    now - lastConvReorderAt >= REORDER_COOLDOWN_MS
  ) {
    lastConvReorderAt = now;
    lastConvOrderIds = sortedItems.map((item) => item.id);
    lastConvActionIds = currentActionIds;
    lastConvRunningIds = currentRunningIds;
    if (pendingConvReorderTimer) {
      clearTimeout(pendingConvReorderTimer);
      pendingConvReorderTimer = null;
    }
    return sortedItems;
  }

  const newById = new Map(sortedItems.map((item) => [item.id, item]));
  const prevIdSet = new Set(lastConvOrderIds);
  const preserved = [];
  for (const id of lastConvOrderIds) {
    const item = newById.get(id);
    if (item) preserved.push(item);
  }
  const added = sortedItems.filter((item) => !prevIdSet.has(item.id));
  const isPinned = (item) => Boolean(item.annotations?.pinned);
  const buffered = [
    ...added.filter(isConversationNeedsAction),
    ...preserved.filter(isConversationNeedsAction),
    ...added.filter((item) => !isConversationNeedsAction(item) && isPinned(item)),
    ...preserved.filter((item) => !isConversationNeedsAction(item) && isPinned(item)),
    ...added.filter((item) => !isConversationNeedsAction(item) && !isPinned(item) && isConversationRunning(item)),
    ...preserved.filter((item) => !isConversationNeedsAction(item) && !isPinned(item) && isConversationRunning(item)),
    ...added.filter((item) => !isConversationNeedsAction(item) && !isPinned(item) && !isConversationRunning(item)),
    ...preserved.filter((item) => !isConversationNeedsAction(item) && !isPinned(item) && !isConversationRunning(item)),
  ];

  lastConvOrderIds = buffered.map((item) => item.id);
  lastConvActionIds = currentActionIds;
  lastConvRunningIds = currentRunningIds;

  const orderChanged =
    buffered.length === sortedItems.length &&
    buffered.some((item, idx) => item.id !== sortedItems[idx].id);
  if (orderChanged) {
    if (!pendingConvReorderTimer) {
      const remaining = Math.max(50, REORDER_COOLDOWN_MS - (now - lastConvReorderAt));
      pendingConvReorderTimer = setTimeout(() => {
        pendingConvReorderTimer = null;
        if (currentTrajectories && isConversationListActive()) {
          renderConversationList(currentTrajectories, { forceReorder: true });
        }
      }, remaining);
    }
  } else if (pendingConvReorderTimer) {
    clearTimeout(pendingConvReorderTimer);
    pendingConvReorderTimer = null;
  }

  return buffered;
}

async function loadConversations(quiet = false) {
  const listEl = document.getElementById("conversations-list");
  refreshGatewayCompat();
  try {
    const data = await rpc("GetAllCascadeTrajectories");
    const summaries = data.trajectorySummaries || {};
    currentTrajectories = summaries;

    renderConversationList(summaries, { forceReorder: !quiet });
  } catch (err) {
    if (quiet) {
      // Background refresh: keep showing the last good list instead of replacing it with an error.
      console.warn("[ListEvents] refresh failed:", err);
      return;
    }
    listEl.innerHTML = `
      <div class="loading-state">
        <p style="color: var(--ios-red);">加载失败: ${escapeHtml(err.message)}</p>
        <button class="btn-ios-secondary" onclick="loadConversations()" style="margin-top:10px;">点击重试</button>
      </div>
    `;
  }
}

window.addEventListener("mgy:languagechange", () => {
  if (currentTrajectories) {
    renderConversationList(currentTrajectories, { forceReorder: false });
  }
});

function formatConvPrBadge(info) {
  if (!info) return "";
  const pr = info.pullRequest;
  if (pr && (pr.number || pr.state)) {
    const st = String(pr.state || "OPEN").toUpperCase();
    const tone = st === "MERGED" ? "merged" : st === "DRAFT" ? "draft" : st === "CLOSED" ? "closed" : "open";
    const num = pr.number ? `#${Number(pr.number)}` : "PR";
    return `<span class="conv-pr-pill ${tone}" title="${escapeHtml(pr.title || "")}">${escapeHtml(num)} · ${escapeHtml(st)}</span>`;
  }
  const br = String(info.currentBranch || "").trim();
  if (br && br !== "HEAD") {
    return `<span class="conv-branch-pill">⎇ ${escapeHtml(br)}</span>`;
  }
  return "";
}

// 子代理会话不在列表里展示（与原生端一致）：有父会话 / 战斗模式分叉 / 子代理规格 / 嵌套深度 > 0。
function isSubagentSummary(info) {
  if (info && info.isSideQuestion === true) return false;
  const meta = info && info.trajectoryMetadata;
  if (!meta) return false;
  if (typeof meta.parentConversationId === "string" && meta.parentConversationId.trim()) return true;
  if (meta.isBattleModeFork === true) return true;
  if (meta.subagentSpec) return true;
  if (meta.agentScript) return true;
  if (typeof meta.nestingDepth === "number" && meta.nestingDepth > 0) return true;
  return false;
}

function arrangeConversationBranchTree(items) {
  if (!Array.isArray(items) || items.length <= 1) return items || [];
  const allIds = new Set(items.map((it) => it.id));
  const childrenByParent = new Map();
  const roots = [];
  for (const item of items) {
    const parentId = typeof item.branchParentId === "string" ? item.branchParentId.trim() : "";
    if (item.isSideQuestion && parentId && parentId !== item.id && allIds.has(parentId)) {
      if (!childrenByParent.has(parentId)) childrenByParent.set(parentId, []);
      childrenByParent.get(parentId).push(item);
    } else {
      roots.push(item);
    }
  }
  if (childrenByParent.size === 0) return items;
  const result = [];
  const emitted = new Set();
  const appendWithChildren = (node) => {
    if (emitted.has(node.id)) return;
    emitted.add(node.id);
    result.push(node);
    const kids = childrenByParent.get(node.id);
    if (kids) {
      for (const child of kids) appendWithChildren(child);
    }
  };
  for (const r of roots) appendWithChildren(r);
  if (result.length < items.length) {
    for (const it of items) {
      if (!emitted.has(it.id)) result.push(it);
    }
  }
  return result;
}

function isConversationUnread(item) {
  if (!item) return false;
  // 运行中或等待操作时不显示未读蓝点，优先展示状态标签
  if (item.status === "CASCADE_RUN_STATUS_RUNNING") return false;
  if (item.needsInput) return false;
  if (item.annotations?.archived) return false;
  if (item.annotations?.markedAsUnread) return true;

  const lastMod = item.lastModifiedTime ? new Date(item.lastModifiedTime).getTime() : 0;
  if (!lastMod) return false;

  const serverView = item.annotations?.lastUserViewTime
    ? new Date(item.annotations.lastUserViewTime).getTime()
    : 0;

  let localView = 0;
  try {
    const stored = localStorage.getItem(`ag_last_view_${item.id}`);
    if (stored) localView = Number(stored);
  } catch (e) {}

  const effectiveView = Math.max(serverView, localView);
  return lastMod > effectiveView;
}

async function markConversationAsRead(cascadeId) {
  if (!cascadeId) return;
  const nowMs = Date.now();
  const nowIso = new Date(nowMs).toISOString();

  try {
    localStorage.setItem(`ag_last_view_${cascadeId}`, nowMs.toString());
  } catch (e) {}

  // 内存中乐观更新，从会话详情返回列表时立即体现已读状态
  if (currentTrajectories && currentTrajectories[cascadeId]) {
    if (!currentTrajectories[cascadeId].annotations) {
      currentTrajectories[cascadeId].annotations = {};
    }
    currentTrajectories[cascadeId].annotations.lastUserViewTime = nowIso;
    currentTrajectories[cascadeId].annotations.markedAsUnread = false;
  }

  // 通过网关向原生 language_server 上报已读时间与清除未读标记
  try {
    fetch("/api/exa.language_server_pb.LanguageServerService/UpdateConversationAnnotations", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Connect-Protocol-Version": "1",
      },
      body: JSON.stringify({
        cascadeIds: [cascadeId],
        annotations: {
          markedAsUnread: false,
          lastUserViewTime: nowIso,
        },
        mergeAnnotations: true,
      }),
    }).catch(() => {});
  } catch (e) {}
}

let showArchivedOnly = false;
let showStarredOnly = false;
const expandedBranchParentIds = new Set();
const searchCollapsedBranchParentIds = new Set();
const manuallyCollapsedActionBranchParentIds = new Set();
let lastBranchSearchQuery = "";

function groupConversationsWithBranches(items) {
  if (!Array.isArray(items) || items.length === 0) return [];
  const byId = new Map();
  for (const item of items) {
    if (item && item.id && !byId.has(item.id)) byId.set(item.id, item);
  }
  const branchesByRoot = new Map();
  const roots = [];

  const resolveVisibleRootId = (item) => {
    if (!item || !item.isSideQuestion) return null;
    let currentParentId = typeof item.branchParentId === "string" ? item.branchParentId.trim() : "";
    let highestVisibleAncestor = null;
    const visited = new Set([item.id]);
    while (currentParentId && !visited.has(currentParentId)) {
      visited.add(currentParentId);
      const parent = byId.get(currentParentId);
      if (!parent) break;
      highestVisibleAncestor = parent.id;
      if (!parent.isSideQuestion) break;
      currentParentId = typeof parent.branchParentId === "string" ? parent.branchParentId.trim() : "";
    }
    return highestVisibleAncestor;
  };

  for (const item of items) {
    const rootId = resolveVisibleRootId(item);
    if (rootId) {
      if (!branchesByRoot.has(rootId)) branchesByRoot.set(rootId, []);
      branchesByRoot.get(rootId).push(item);
    } else {
      roots.push(item);
    }
  }
  const groups = roots.map((root) => ({
    root,
    branches: branchesByRoot.get(root.id) || [],
  }));
  // 防御性兜底：极端环引用情况下不丢失任何会话
  const emitted = new Set();
  for (const g of groups) {
    emitted.add(g.root.id);
    for (const b of g.branches) emitted.add(b.id);
  }
  if (emitted.size < items.length) {
    for (const item of items) {
      if (item && item.id && !emitted.has(item.id)) {
        emitted.add(item.id);
        groups.push({ root: item, branches: [] });
      }
    }
  }
  return groups;
}

function getBranchAggregateStatus(branches) {
  if (!Array.isArray(branches) || branches.length === 0) return "none";
  if (branches.some((b) => isConversationNeedsAction(b))) return "action";
  if (branches.some((b) => Boolean(b.hasError) || b.status === "CASCADE_RUN_STATUS_ERROR")) return "error";
  if (branches.some((b) => isConversationRunning(b))) return "working";
  if (branches.some((b) => DraftManager.has(b.id))) return "draft";
  if (branches.some((b) => isConversationUnread(b))) return "unread";
  return "none";
}

function isBranchGroupExpanded(parentId, hasAction, isSearching) {
  if (isSearching) return !searchCollapsedBranchParentIds.has(parentId);
  if (hasAction) return !manuallyCollapsedActionBranchParentIds.has(parentId);
  return expandedBranchParentIds.has(parentId);
}

function toggleBranchGroup(parentId, hasAction, isSearching) {
  if (isSearching) {
    if (searchCollapsedBranchParentIds.has(parentId)) {
      searchCollapsedBranchParentIds.delete(parentId);
    } else {
      searchCollapsedBranchParentIds.add(parentId);
    }
    return;
  }
  const currentlyExpanded = isBranchGroupExpanded(parentId, hasAction, false);
  if (currentlyExpanded) {
    expandedBranchParentIds.delete(parentId);
    if (hasAction) manuallyCollapsedActionBranchParentIds.add(parentId);
  } else {
    manuallyCollapsedActionBranchParentIds.delete(parentId);
    expandedBranchParentIds.add(parentId);
  }
}

function renderConversationCardHtml(item, { isAttachedBranchChild = false, isLastBranchChild = false, hasAttachedBranches = false } = {}) {
  const hasError = !!item.hasError || item.status === "CASCADE_RUN_STATUS_ERROR";
  const hasAction = !hasError && !!item.needsInput;
  const isRunning = !hasError && !hasAction && isConversationRunning(item);
  const hasSwarm = Boolean(item.hasSwarm);
  const isSwarmWorking = hasSwarm && (Number(item.swarmWorkingCount || 0) > 0 || isRunning);
  const swarmBeeHtml =
    hasSwarm && typeof SwarmManager !== "undefined" && typeof SwarmManager.renderListBeeBadge === "function"
      ? SwarmManager.renderListBeeBadge(isSwarmWorking)
      : "";
  const hasDraft = !hasError && !hasAction && !isRunning && DraftManager.has(item.id);
  const isUnread = !hasError && !isRunning && !hasAction && !hasDraft && isConversationUnread(item);
  const unreadDotHtml = isUnread
    ? `<div class="status-unread-dot" title="${escapeHtml(typeof t === "function" ? t("未读新消息") : "未读新消息")}" data-testid="status-unread-dot"><div class="dot-halo"></div><div class="dot-core"></div></div>`
    : "";
  const badgeHtml = hasError
    ? `<span class="badge badge-error">error</span>`
    : (hasAction
      ? `<span class="badge badge-action">ACTION</span>`
      : (isRunning
        ? `<span class="badge badge-running">RUNNING</span>`
        : (hasDraft
          ? `<span class="badge badge-draft">DRAFT</span>`
          : unreadDotHtml)));
  const title = formatConversationTitle(item.annotations, item.summary, typeof t === "function" ? t("未命名会话") : "未命名会话");
  const isPinned = Boolean(item.annotations?.pinned);
  const pinIconHtml = isPinned
    ? `<svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor" class="conv-pin-icon" title="${typeof t === "function" ? t("已置顶") : "已置顶"}" style="color: #f59e0b; margin-right: 4px; flex-shrink: 0;"><path d="M16 12V4h1V2H7v2h1v8l-2 2v2h5.2v6h1.6v-6H18v-2l-2-2z"/></svg>`
    : "";
  const isStarred = Boolean(item.annotations?.starred);
  const starIconHtml = isStarred
    ? `<span class="conv-star-icon" title="${typeof t === "function" ? t("已星标") : "已星标"}" aria-label="${typeof t === "function" ? t("已星标") : "已星标"}">★</span>`
    : "";
  const wsUri = item.workspaceUris?.[0] || item.workspaces?.[0]?.workspaceFolderAbsoluteUri || "";
  const wsName = item.workspaceName || wsUri.split("/").filter(Boolean).pop() || "Chat";
  const timeStr = formatRelativeTime(item.lastModifiedTime);
  const stepCount = item.stepCount || 0;
  const stepLabel = (window.I18n && window.I18n.isZh && window.I18n.isZh())
    ? "步骤"
    : ((window.I18n && window.I18n.isJa && window.I18n.isJa())
        ? "ステップ"
        : (stepCount === 1 ? "step" : "steps"));

  const idEsc = escapeHtml(item.id || "");
  const isSideQuestion = Boolean(item.isSideQuestion);
  const isArchived = Boolean(item.annotations?.archived);
  const archiveLabel = isArchived
    ? (typeof t === "function" ? t("取消归档") : "取消归档")
    : (typeof t === "function" ? t("归档会话") : "归档会话");

  const actionsHtml = `
    <div class="conv-card-actions">
      <button class="conv-card-archive-btn${isArchived ? " is-unarchive" : ""}" type="button" aria-label="${archiveLabel}" title="${archiveLabel}" data-id="${idEsc}" data-archived="${isArchived ? "1" : "0"}">
        <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="21 8 21 21 3 21 3 8"></polyline>
          <rect x="1" y="3" width="22" height="5"></rect>
          <line x1="10" y1="12" x2="14" y2="12"></line>
        </svg>
      </button>
      <button class="conv-card-delete-btn" type="button" aria-label="${typeof t === "function" ? t("删除会话") : "删除会话"}" data-id="${idEsc}">
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="3 6 5 6 21 6"></polyline>
          <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path>
        </svg>
      </button>
    </div>
  `;

  if (isSideQuestion && isAttachedBranchChild) {
    const wrapperClasses = [
      "conv-card-wrapper",
      "is-side-question",
      "is-branch-embedded",
      isLastBranchChild ? "is-branch-last" : "",
    ].filter(Boolean).join(" ");
    return `
      <div class="${wrapperClasses}" data-id="${idEsc}">
        ${actionsHtml}
        <div class="conv-card conv-branch-embedded-card" data-id="${idEsc}" data-title="${escapeHtml(title)}">
          ${pinIconHtml}${starIconHtml}
          <span class="conv-branch-compact-title" data-i18n-skip="1">${escapeHtml(title)}</span>
          <span class="conv-branch-compact-right">
            ${timeStr ? `<span class="conv-branch-compact-time">${escapeHtml(timeStr)}</span>` : ""}
            ${badgeHtml}
          </span>
        </div>
      </div>
    `;
  }

  const wrapperClasses = [
    "conv-card-wrapper",
    isSideQuestion ? "is-side-question" : "",
    hasAttachedBranches ? "has-branch-toggle" : "",
  ].filter(Boolean).join(" ");
  const branchPillHtml = isSideQuestion
    ? `<span class="conv-branch-tag">${renderSideQuestionIconHtml(11)}<span>${typeof t === "function" ? t("旁路") : "旁路"}</span></span>`
    : "";
  const scheduledIconHtml = item.isScheduledTask && wsName !== "Chat" ? renderScheduledAlarmIconHtml() : "";
  const subMetaHtml = isSideQuestion && item.branchParentTitle
    ? `<span class="ws-name">${typeof t === "function" ? t("来自") : "来自"}「<span data-i18n-skip="1">${escapeHtml(item.branchParentTitle)}</span>」</span>`
    : `${wsName === "Chat"
        ? `<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"></path>
          </svg>`
        : `<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"></path>
          </svg>`}
      <span class="ws-name" data-i18n-skip="1">${escapeHtml(wsName)}</span>${scheduledIconHtml}${formatConvPrBadge(item)}`;
  const rightMetaHtml = isSideQuestion ? escapeHtml(timeStr) : `${stepCount} ${stepLabel} • ${timeStr}`;

  return `
    <div class="${wrapperClasses}" data-id="${idEsc}">
      ${actionsHtml}
      <div class="conv-card" data-id="${idEsc}" data-title="${escapeHtml(title)}">
        <div class="conv-card-top">
          <div class="conv-title">${pinIconHtml}${starIconHtml}${swarmBeeHtml}${branchPillHtml}<span data-i18n-skip="1">${escapeHtml(title)}</span></div>
          ${badgeHtml}
        </div>
        <div class="conv-card-bottom">
          <div class="conv-meta">
            ${subMetaHtml}
          </div>
          <span class="conv-steps-time">${rightMetaHtml}</span>
        </div>
      </div>
    </div>
  `;
}

function renderBranchDrawerHtml(parentId, branches, isExpanded, aggStatus) {
  const parentIdEsc = escapeHtml(parentId || "");
  const count = branches.length;
  const rawCountLabel = count === 1 ? "1 个旁路" : `${count} 个旁路`;
  const countLabel = typeof t === "function" ? t(rawCountLabel) : rawCountLabel;
  const collapseLabel = typeof t === "function" ? t("收起") : "收起";
  const firstBranch = branches[0];
  const previewTitle = firstBranch
    ? formatConversationTitle(firstBranch.annotations, firstBranch.summary, "")
    : "";
  const hasAction = aggStatus === "action";

  let aggStatusHtml = "";
  if (aggStatus === "action") {
    aggStatusHtml = `<span class="badge badge-action conv-branch-toggle-badge">ACTION</span>`;
  } else if (aggStatus === "error") {
    aggStatusHtml = `<span class="badge badge-error conv-branch-toggle-badge">error</span>`;
  } else if (aggStatus === "working") {
    aggStatusHtml = `<span class="badge badge-running conv-branch-toggle-badge">RUNNING</span>`;
  } else if (aggStatus === "draft") {
    aggStatusHtml = `<span class="badge badge-draft conv-branch-toggle-badge">DRAFT</span>`;
  } else if (aggStatus === "unread") {
    aggStatusHtml = `<div class="status-unread-dot conv-branch-toggle-dot" title="${escapeHtml(typeof t === "function" ? t("未读新消息") : "未读新消息")}"><div class="dot-halo"></div><div class="dot-core"></div></div>`;
  }

  const childrenHtml = branches
    .map((child, idx) => {
      const isLast = idx === branches.length - 1;
      const cardHtml = renderConversationCardHtml(child, {
        isAttachedBranchChild: true,
        isLastBranchChild: isLast,
        hasAttachedBranches: false,
      });
      return `<div class="conv-branch-child-slot${isLast ? " is-branch-last" : ""}">${cardHtml}</div>`;
    })
    .join("");

  return `
    <div class="conv-branch-drawer${isExpanded ? " is-expanded" : ""}" data-branch-parent="${parentIdEsc}">
      <button type="button" class="conv-branch-toggle-btn" data-branch-parent="${parentIdEsc}" data-has-action="${hasAction ? "1" : "0"}" aria-expanded="${isExpanded ? "true" : "false"}">
        <span class="conv-branch-toggle-icon" aria-hidden="true">${renderSideQuestionIconHtml(12)}</span>
        <span class="conv-branch-toggle-count">${escapeHtml(countLabel)}</span>
        ${previewTitle ? `<span class="conv-branch-toggle-preview-wrap"><span class="conv-branch-toggle-sep" aria-hidden="true">·</span><span class="conv-branch-toggle-preview" data-i18n-skip="1">${escapeHtml(previewTitle)}</span></span>` : ""}
        <span class="conv-branch-toggle-spacer"></span>
        ${aggStatusHtml ? `<span class="conv-branch-toggle-agg">${aggStatusHtml}</span>` : ""}
        <span class="conv-branch-toggle-collapse">${escapeHtml(collapseLabel)}</span>
        <svg class="conv-branch-toggle-chevron" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <polyline points="6 9 12 15 18 9"></polyline>
        </svg>
      </button>
      <div class="conv-branch-drawer-body"${isExpanded ? "" : " inert"}>
        <div class="conv-branch-drawer-inner">
          ${childrenHtml}
        </div>
      </div>
    </div>
  `;
}

function renderConversationList(summaries, { forceReorder = false } = {}) {
  const listEl = document.getElementById("conversations-list");
  const searchInput = document.getElementById("conv-search");
  const clearBtn = document.getElementById("btn-search-clear");
  const query = (searchInput?.value || "").toLowerCase().trim();

  if (clearBtn) {
    if (query) {
      clearBtn.classList.remove("hidden");
    } else {
      clearBtn.classList.add("hidden");
    }
  }

  const allNonSubagents = Object.entries(summaries)
    .filter(([, info]) => !isSubagentSummary(info))
    .map(([id, info]) => ({ id, ...info }));

  const archivedCount = allNonSubagents.filter((it) => Boolean(it.annotations?.archived)).length;
  const starredCount = allNonSubagents.filter((it) => !it.annotations?.archived && Boolean(it.annotations?.starred)).length;
  if (showArchivedOnly && archivedCount === 0) {
    showArchivedOnly = false;
  }
  if (showStarredOnly && starredCount === 0) {
    showStarredOnly = false;
  }

  const sortedAll = allNonSubagents
    .filter((it) => {
      if (showArchivedOnly) return Boolean(it.annotations?.archived);
      if (showStarredOnly) return !it.annotations?.archived && Boolean(it.annotations?.starred);
      return !it.annotations?.archived;
    })
    .sort((a, b) => {
      const aAction = isConversationNeedsAction(a);
      const bAction = isConversationNeedsAction(b);
      if (aAction !== bAction) return aAction ? -1 : 1;
      const aPinned = Boolean(a.annotations?.pinned);
      const bPinned = Boolean(b.annotations?.pinned);
      if (aPinned !== bPinned) return aPinned ? -1 : 1;
      const aRunning = isConversationRunning(a);
      const bRunning = isConversationRunning(b);
      if (aRunning !== bRunning) return aRunning ? -1 : 1;
      return new Date(b.lastModifiedTime || 0) - new Date(a.lastModifiedTime || 0);
    });

  const items = arrangeConversationBranchTree(
    applyConversationReorderBuffer(sortedAll, forceReorder)
      .filter((item) => {
        if (!query) return true;
        const title = formatConversationTitle(item.annotations, item.summary, "").toLowerCase();
        const ws = (item.workspaceUris?.[0] || item.workspaceName || "").toLowerCase();
        const parentTitle = (item.branchParentTitle || "").toLowerCase();
        return title.includes(query) || ws.includes(query) || parentTitle.includes(query) || item.id.includes(query);
      })
  );

  const archivedBannerHtml = showArchivedOnly
    ? `<div class="conv-archived-banner" style="display:flex;align-items:center;justify-content:space-between;padding:8px 12px;margin:0 0 8px;border-radius:12px;background:var(--surface-secondary,rgba(128,128,128,0.12));font-size:12.5px;">
        <span style="font-weight:600;">已归档会话 (${archivedCount})</span>
        <button type="button" class="conv-exit-archived-btn" style="border:none;background:none;color:var(--accent,#6366f1);font-weight:600;cursor:pointer;font-size:12.5px;">返回全部</button>
      </div>`
    : (showStarredOnly
      ? `<div class="conv-archived-banner" style="display:flex;align-items:center;justify-content:space-between;padding:8px 12px;margin:0 0 8px;border-radius:12px;background:rgba(245,158,11,0.12);font-size:12.5px;">
          <span style="font-weight:600;color:#f59e0b;">${escapeHtml(typeof t === "function" ? t(`★ 已星标 (${starredCount})`) : `★ 已星标 (${starredCount})`)}</span>
          <button type="button" class="conv-exit-starred-btn" style="border:none;background:none;color:var(--accent,#6366f1);font-weight:600;cursor:pointer;font-size:12.5px;">返回全部</button>
        </div>`
      : "");

  const starredFooterHtml = !showArchivedOnly && !showStarredOnly && !query && starredCount > 0
    ? `<button type="button" class="conv-open-starred-btn" style="display:flex;align-items:center;justify-content:center;gap:6px;width:100%;padding:8px 12px;margin-top:6px;border:none;background:transparent;color:#f59e0b;font-size:12.5px;font-weight:500;cursor:pointer;">
        ${escapeHtml(typeof t === "function" ? t(`★ 已星标 (${starredCount}) ›`) : `★ 已星标 (${starredCount}) ›`)}
      </button>`
    : "";

  const archivedFooterHtml = starredFooterHtml + (!showArchivedOnly && !showStarredOnly && !query && archivedCount > 0
    ? `<button type="button" class="conv-open-archived-btn" style="display:flex;align-items:center;justify-content:center;gap:6px;width:100%;padding:10px 12px;margin-top:2px;border:none;background:transparent;color:var(--text-secondary,#8e8e93);font-size:12.5px;font-weight:500;cursor:pointer;">
        已归档会话 (${archivedCount}) ›
      </button>`
    : "");

  if (items.length === 0) {
    // 有内容命中（或正在搜索）时不显示「暂无匹配」，由下面的内容命中区承接
    const contentBusy = !!query && contentSearch.query === (searchInput?.value || "").trim() && (contentSearch.loading || contentSearch.results.length > 0);
    listEl.innerHTML = archivedBannerHtml + `
      <div class="loading-state no-match-state${contentBusy ? " hidden" : ""}">
        <p>${showArchivedOnly ? "暂无已归档会话" : "暂无匹配会话"}</p>
      </div>
    ` + archivedFooterHtml;
    attachConversationCardInteractions();
    return;
  }

  if (query !== lastBranchSearchQuery) {
    lastBranchSearchQuery = query;
    searchCollapsedBranchParentIds.clear();
  }

  const isSearching = Boolean(query);
  const groups = groupConversationsWithBranches(items);
  const htmlParts = [];
  for (const group of groups) {
    const hasBranches = group.branches.length > 0;
    const rootHtml = renderConversationCardHtml(group.root, {
      isAttachedBranchChild: false,
      isLastBranchChild: false,
      hasAttachedBranches: hasBranches,
    });
    if (hasBranches) {
      const aggStatus = getBranchAggregateStatus(group.branches);
      const isExpanded = isBranchGroupExpanded(group.root.id, aggStatus === "action", isSearching);
      htmlParts.push(
        `<div class="conv-branch-group">${rootHtml}${renderBranchDrawerHtml(group.root.id, group.branches, isExpanded, aggStatus)}</div>`
      );
    } else {
      htmlParts.push(rootHtml);
    }
  }
  listEl.innerHTML = archivedBannerHtml + htmlParts.join("") + archivedFooterHtml;

  attachConversationCardInteractions();
  updateAppBadgeFromList(items);
}

// --- iOS Conversation List Card Interactions (Swipe-to-Delete & Long-Press Rename via Event Delegation) ---

let convDelegationInitialized = false;
let activeTouchState = null;
let touchHandled = false;

function closeCard(card) {
  if (!card) return;
  card.style.transform = "translateX(0)";
  card.classList.remove("swiped", "swiping");
  card.closest(".conv-card-wrapper")?.classList.remove("swiped", "swiping");
}

function closeOtherCards(exceptCard) {
  document.querySelectorAll(".conv-card.swiped, .conv-card.swiping").forEach((c) => {
    if (c !== exceptCard) {
      c.style.transform = "translateX(0)";
      c.classList.remove("swiped", "swiping");
      c.closest(".conv-card-wrapper")?.classList.remove("swiped", "swiping");
    }
  });
}

function initConversationCardDelegation() {
  const listEl = document.getElementById("conversations-list");
  if (!listEl || convDelegationInitialized) return;
  convDelegationInitialized = true;

  listEl.addEventListener("touchstart", (e) => {
    if (e.touches.length > 1) return;
    if (e.target.closest(".conv-card-delete-btn") || e.target.closest(".conv-card-archive-btn")) return;
    const card = e.target.closest(".conv-card");
    if (!card) return;
    const wrapper = card.closest(".conv-card-wrapper");
    if (!wrapper) return;
    const id = wrapper.getAttribute("data-id");

    const startX = e.touches[0].clientX;
    const startY = e.touches[0].clientY;

    card.classList.remove("swiping");
    wrapper.classList.remove("swiping");

    const longPressTimer = setTimeout(() => {
      if (activeTouchState && !activeTouchState.hasMoved && !activeTouchState.isDragging &&
          Math.hypot(activeTouchState.currentX - startX, activeTouchState.currentY - startY) < 10) {
        activeTouchState.hasTriggeredLongPress = true;
        triggerHaptic("medium");
        const title = card.getAttribute("data-title") || "会话";
        if (window.Phase4?.openConvContextMenu) {
          window.Phase4.openConvContextMenu(id, title);
        } else {
          openRenameAlert(id, title);
        }
      }
    }, 450);

    activeTouchState = {
      card,
      wrapper,
      id,
      startX,
      startY,
      currentX: startX,
      currentY: startY,
      isDragging: false,
      isHorizontal: null,
      hasMoved: false,
      longPressTimer,
      hasTriggeredLongPress: false,
      touchStartTime: Date.now()
    };
  }, { passive: true });

  listEl.addEventListener("touchmove", (e) => {
    if (!activeTouchState) return;
    const { card, wrapper, startX, startY } = activeTouchState;
    activeTouchState.currentX = e.touches[0].clientX;
    activeTouchState.currentY = e.touches[0].clientY;
    const dx = activeTouchState.currentX - startX;
    const dy = activeTouchState.currentY - startY;

    if (!activeTouchState.hasMoved && (Math.abs(dx) > 7 || Math.abs(dy) > 7)) {
      activeTouchState.hasMoved = true;
      clearTimeout(activeTouchState.longPressTimer);
    }

    if (activeTouchState.isHorizontal === null && activeTouchState.hasMoved) {
      activeTouchState.isHorizontal = Math.abs(dx) > Math.abs(dy) * 1.2;
      if (!activeTouchState.isHorizontal) {
        clearTimeout(activeTouchState.longPressTimer);
      }
    }

    if (activeTouchState.isHorizontal) {
      clearTimeout(activeTouchState.longPressTimer);
      closeOtherCards(card);
      activeTouchState.isDragging = true;
      card.classList.add("swiping");
      wrapper.classList.add("swiping");

      const swipeMax = wrapper.classList.contains("is-branch-embedded") ? 96 : 128;
      const isAlreadySwiped = card.classList.contains("swiped");
      const baseOffset = isAlreadySwiped ? -swipeMax : 0;
      let newX = baseOffset + dx;

      if (newX > 0) {
        newX = newX * 0.2;
      } else if (newX < -swipeMax) {
        newX = -swipeMax + (newX + swipeMax) * 0.25;
      }
      card.style.transform = `translateX(${newX}px)`;
    }
  }, { passive: true });

  const cancelActiveTouch = () => {
    if (!activeTouchState) return;
    clearTimeout(activeTouchState.longPressTimer);
    activeTouchState.card.classList.remove("swiping");
    activeTouchState.wrapper.classList.remove("swiping");
    activeTouchState = null;
  };

  listEl.addEventListener("touchcancel", cancelActiveTouch);

  listEl.addEventListener("touchend", () => {
    if (!activeTouchState) return;
    const { card, wrapper, id, startX, currentX, isDragging, isHorizontal, hasMoved,
            hasTriggeredLongPress, touchStartTime, longPressTimer } = activeTouchState;
    clearTimeout(longPressTimer);
    card.classList.remove("swiping");
    wrapper.classList.remove("swiping");

    if (hasTriggeredLongPress) {
      activeTouchState = null;
      return;
    }

    if (isDragging && isHorizontal) {
      const dx = currentX - startX;
      const swipeMax = wrapper.classList.contains("is-branch-embedded") ? 96 : 128;
      const openThreshold = wrapper.classList.contains("is-branch-embedded") ? -36 : -44;
      const isAlreadySwiped = card.classList.contains("swiped");
      if (isAlreadySwiped) {
        if (dx > 25) {
          closeCard(card);
        } else {
          card.style.transform = `translateX(-${swipeMax}px)`;
          card.classList.add("swiped");
          wrapper.classList.add("swiped");
        }
      } else {
        if (dx < openThreshold) {
          card.style.transform = `translateX(-${swipeMax}px)`;
          card.classList.add("swiped");
          wrapper.classList.add("swiped");
          triggerHaptic("light");
        } else {
          closeCard(card);
        }
      }
      activeTouchState = null;
      return;
    }

    activeTouchState = null;

    if (hasMoved) return;
    if (Date.now() - touchStartTime > 600) return;

    touchHandled = true;
    setTimeout(() => { touchHandled = false; }, 400);

    if (card.classList.contains("swiped")) {
      closeCard(card);
      return;
    }

    const anySwiped = document.querySelectorAll(".conv-card.swiped, .conv-card-wrapper.swiped");
    if (anySwiped.length > 0) {
      closeOtherCards();
      return;
    }

    closeOtherCards();
    triggerHaptic("selection");
    navigateTo(`#c=${id}`);
  });

  listEl.addEventListener("click", (e) => {
    const branchToggleBtn = e.target.closest(".conv-branch-toggle-btn");
    if (branchToggleBtn) {
      e.stopPropagation();
      closeOtherCards();
      triggerHaptic("light");
      const parentId = branchToggleBtn.getAttribute("data-branch-parent");
      const hasAction = branchToggleBtn.getAttribute("data-has-action") === "1";
      const q = (document.getElementById("conv-search")?.value || "").toLowerCase().trim();
      const isSearching = Boolean(q);
      if (parentId) {
        toggleBranchGroup(parentId, hasAction, isSearching);
        const drawer = branchToggleBtn.closest(".conv-branch-drawer");
        if (drawer) {
          const nowExpanded = isBranchGroupExpanded(parentId, hasAction, isSearching);
          drawer.classList.toggle("is-expanded", nowExpanded);
          branchToggleBtn.setAttribute("aria-expanded", nowExpanded ? "true" : "false");
          const bodyEl = drawer.querySelector(":scope > .conv-branch-drawer-body");
          if (bodyEl) {
            bodyEl.inert = !nowExpanded;
          }
        } else {
          renderConversationList(currentTrajectories || {}, { forceReorder: false });
        }
      }
      return;
    }
    const openStarBtn = e.target.closest(".conv-open-starred-btn");
    if (openStarBtn) {
      e.stopPropagation();
      showStarredOnly = true;
      showArchivedOnly = false;
      renderConversationList(currentTrajectories || {}, { forceReorder: true });
      return;
    }
    const exitStarBtn = e.target.closest(".conv-exit-starred-btn");
    if (exitStarBtn) {
      e.stopPropagation();
      showStarredOnly = false;
      renderConversationList(currentTrajectories || {}, { forceReorder: true });
      return;
    }
    const openArchBtn = e.target.closest(".conv-open-archived-btn");
    if (openArchBtn) {
      e.stopPropagation();
      showArchivedOnly = true;
      showStarredOnly = false;
      renderConversationList(currentTrajectories || {}, { forceReorder: true });
      return;
    }
    const exitArchBtn = e.target.closest(".conv-exit-archived-btn");
    if (exitArchBtn) {
      e.stopPropagation();
      showArchivedOnly = false;
      renderConversationList(currentTrajectories || {}, { forceReorder: true });
      return;
    }
    const archiveBtn = e.target.closest(".conv-card-archive-btn");
    if (archiveBtn) {
      e.stopPropagation();
      triggerHaptic("medium");
      const id = archiveBtn.getAttribute("data-id") || archiveBtn.closest(".conv-card-wrapper")?.getAttribute("data-id");
      const isCurrentlyArchived = archiveBtn.getAttribute("data-archived") === "1";
      if (id) openArchiveActionSheet(id, !isCurrentlyArchived);
      return;
    }
    const deleteBtn = e.target.closest(".conv-card-delete-btn");
    if (deleteBtn) {
      e.stopPropagation();
      triggerHaptic("medium");
      const id = deleteBtn.getAttribute("data-id") || deleteBtn.closest(".conv-card-wrapper")?.getAttribute("data-id");
      if (id) openDeleteActionSheet(id);
      return;
    }

    if (touchHandled) return;
    const card = e.target.closest(".conv-card");
    if (!card) return;
    const wrapper = card.closest(".conv-card-wrapper");
    const id = wrapper?.getAttribute("data-id");
    if (!id) return;

    if (card.classList.contains("swiped")) {
      closeCard(card);
      return;
    }
    const anySwiped = document.querySelectorAll(".conv-card.swiped, .conv-card-wrapper.swiped");
    if (anySwiped.length > 0) {
      closeOtherCards();
      return;
    }
    closeOtherCards();
    triggerHaptic("selection");
    navigateTo(`#c=${id}`);
  });

  listEl.addEventListener("contextmenu", (e) => {
    const card = e.target.closest(".conv-card");
    if (!card) return;
    const wrapper = card.closest(".conv-card-wrapper");
    const id = wrapper?.getAttribute("data-id");
    if (!id) return;
    e.preventDefault();
    const title = card.getAttribute("data-title") || "会话";
    if (window.Phase4?.openConvContextMenu) {
      window.Phase4.openConvContextMenu(id, title);
    } else {
      openRenameAlert(id, title);
    }
  });
}

function attachConversationCardInteractions() {
  initConversationCardDelegation();
}

// --- Delete Conversation ActionSheet ---

let pendingDeleteCascadeId = null;

function openDeleteActionSheet(id) {
  pendingDeleteCascadeId = id;
  const sheet = document.getElementById("actionsheet-delete");
  if (sheet) sheet.classList.remove("hidden");
}

// --- Archive / Unarchive Conversation ActionSheet ---

let pendingArchiveCascadeId = null;
let pendingArchiveTargetState = true;

function openArchiveActionSheet(id, willArchive) {
  closeOtherCards();
  pendingArchiveCascadeId = id;
  pendingArchiveTargetState = !!willArchive;
  const sheet = document.getElementById("actionsheet-archive");
  const titleEl = document.getElementById("actionsheet-archive-title");
  const subtitleEl = document.getElementById("actionsheet-archive-subtitle");
  const confirmBtn = document.getElementById("btn-actionsheet-archive-confirm");
  if (titleEl) {
    titleEl.textContent = pendingArchiveTargetState
      ? "确定归档此会话吗？"
      : "确定取消归档此会话吗？";
  }
  if (subtitleEl) {
    if (pendingArchiveTargetState) {
      subtitleEl.textContent = "归档后该会话将移入「已归档」，随时可查看或恢复。";
      subtitleEl.classList.remove("hidden");
    } else {
      subtitleEl.textContent = "";
      subtitleEl.classList.add("hidden");
    }
  }
  if (confirmBtn) {
    confirmBtn.textContent = pendingArchiveTargetState ? "归档会话" : "取消归档";
  }
  if (sheet) {
    if (window.I18n && typeof window.I18n.translateDOM === "function") {
      window.I18n.translateDOM(sheet, true);
    }
    sheet.classList.remove("hidden");
  }
}

function closeArchiveActionSheet() {
  pendingArchiveCascadeId = null;
  const sheet = document.getElementById("actionsheet-archive");
  if (sheet) sheet.classList.add("hidden");
}

async function confirmArchiveConversation() {
  const id = pendingArchiveCascadeId;
  const willArchive = pendingArchiveTargetState;
  closeArchiveActionSheet();
  if (!id) return;
  await toggleArchiveConversation(id, willArchive);
}

async function toggleArchiveConversation(id, archived) {
  if (!id) return;
  closeOtherCards();
  const prev = currentTrajectories?.[id]?.annotations?.archived;
  if (currentTrajectories && currentTrajectories[id]) {
    if (!currentTrajectories[id].annotations) currentTrajectories[id].annotations = {};
    currentTrajectories[id].annotations.archived = archived;
    renderConversationList(currentTrajectories, { forceReorder: true });
  }
  try {
    await rpc("UpdateConversationAnnotations", {
      cascadeIds: [id],
      annotations: { archived },
      mergeAnnotations: true,
    });
    showToast?.(archived ? "已归档会话" : "已取消归档");
  } catch (err) {
    console.error("Failed to update archive state:", err);
    if (currentTrajectories && currentTrajectories[id]?.annotations) {
      currentTrajectories[id].annotations.archived = prev;
      renderConversationList(currentTrajectories, { forceReorder: true });
    }
    showToast?.("归档操作失败", 2600);
  }
}

function closeDeleteActionSheet() {
  pendingDeleteCascadeId = null;
  const sheet = document.getElementById("actionsheet-delete");
  if (sheet) sheet.classList.add("hidden");
}

async function confirmDeleteConversation() {
  const id = pendingDeleteCascadeId;
  closeDeleteActionSheet();
  if (!id) return;

  const safeIdSel = typeof CSS !== "undefined" && CSS.escape ? CSS.escape(id) : escapeHtml(id);
  const wrapper = document.querySelector(`.conv-card-wrapper[data-id="${safeIdSel}"]`);
  if (wrapper) {
    wrapper.classList.add("deleting");
    setTimeout(() => wrapper.remove(), 280);
  }

  delete currentTrajectories[id];
  delete sessionStepsCache[id];
  PersistentStepsCache.delete(id);
  const lruIdx = sessionStepsLRU.indexOf(id);
  if (lruIdx !== -1) sessionStepsLRU.splice(lruIdx, 1);
  DraftManager.clear(id);

  try {
    await rpc("DeleteCascadeTrajectory", { cascadeId: id });
    triggerHaptic("heavy");
  } catch (err) {
    console.error("Failed to delete conversation on server:", err);
    loadConversations();
  }
}

// --- Rename Conversation Alert Dialog ---

let pendingRenameCascadeId = null;

function openRenameAlert(id, currentTitle) {
  pendingRenameCascadeId = id;
  const overlay = document.getElementById("alert-rename");
  const input = document.getElementById("input-rename-title");
  if (input) {
    input.value = currentTitle || "";
  }
  if (overlay) overlay.classList.remove("hidden");
  setTimeout(() => {
    input?.focus();
    input?.select();
  }, 60);
}

function closeRenameAlert() {
  pendingRenameCascadeId = null;
  const overlay = document.getElementById("alert-rename");
  if (overlay) overlay.classList.add("hidden");
}

async function submitRenameConversation() {
  const id = pendingRenameCascadeId;
  const input = document.getElementById("input-rename-title");
  const newTitle = input?.value?.trim();
  closeRenameAlert();

  if (!id || !newTitle) return;

  if (currentTrajectories[id]) {
    if (!currentTrajectories[id].annotations) currentTrajectories[id].annotations = {};
    currentTrajectories[id].annotations.title = newTitle;
  }
  const safeIdSel = typeof CSS !== "undefined" && CSS.escape ? CSS.escape(id) : escapeHtml(id);
  const titleEl = document.querySelector(`.conv-card[data-id="${safeIdSel}"] .conv-title`);
  if (titleEl) titleEl.textContent = newTitle;
  const cardEl = document.querySelector(`.conv-card[data-id="${safeIdSel}"]`);
  if (cardEl) cardEl.setAttribute("data-title", newTitle);

  try {
    await rpc("UpdateConversationAnnotations", {
      cascadeIds: [id],
      annotations: { title: newTitle },
      mergeAnnotations: true,
    });
    triggerHaptic("success");
  } catch (err) {
    console.error("Failed to rename conversation:", err);
    loadConversations();
  }
}

// --- iOS Pull-to-Refresh Gesture ---

function initPullToRefresh() {
  const listEl = document.getElementById("conversations-list");
  const refreshBar = document.getElementById("pull-refresh-bar");
  if (!listEl || !refreshBar) return;

  let startY = 0;
  let isPulling = false;
  let pullDistance = 0;

  listEl.addEventListener("touchstart", (e) => {
    if (listEl.scrollTop <= 0) {
      startY = e.touches[0].clientY;
      isPulling = true;
      pullDistance = 0;
    } else {
      isPulling = false;
    }
  }, { passive: true });

  listEl.addEventListener("touchmove", (e) => {
    if (!isPulling) return;
    const currentY = e.touches[0].clientY;
    const dy = currentY - startY;

    if (dy > 0 && listEl.scrollTop <= 0) {
      pullDistance = Math.min(75, dy * 0.45);
      refreshBar.classList.add("pulling");
      refreshBar.style.height = `${pullDistance}px`;
      refreshBar.style.opacity = `${Math.min(1, pullDistance / 35)}`;
      refreshBar.style.transform = `translateY(${pullDistance - 22}px)`;
    } else {
      pullDistance = 0;
      refreshBar.style.height = "0";
      refreshBar.style.opacity = "0";
    }
  }, { passive: true });

  listEl.addEventListener("touchend", async () => {
    if (!isPulling) return;
    isPulling = false;
    refreshBar.classList.remove("pulling");

    if (pullDistance >= 40) {
      refreshBar.classList.add("refreshing");
      refreshBar.style.height = "48px";
      refreshBar.style.opacity = "1";
      refreshBar.style.transform = "translateY(0)";
      triggerHaptic("light");

      try {
        await loadConversations();
      } finally {
        setTimeout(() => {
          refreshBar.classList.remove("refreshing");
          refreshBar.style.height = "0";
          refreshBar.style.opacity = "0";
        }, 260);
      }
    } else {
      refreshBar.style.height = "0";
      refreshBar.style.opacity = "0";
    }
  });
}

// --- iOS Edge Swipe Back Gesture ---

function initEdgeSwipeBack() {
  const chatView = document.getElementById("view-chat");
  const convView = document.getElementById("view-conversations");
  if (!chatView || !convView) return;

  let startX = 0;
  let startY = 0;
  let currentX = 0;
  let isSwiping = false;
  let isHorizontal = null;
  let startTime = 0;

  window.addEventListener("touchstart", (e) => {
    if ((!activeCascadeId && !activeDraftSession) || e.touches.length > 1) return;
    if (e.target && typeof e.target.closest === "function" && e.target.closest("textarea, input, select, [contenteditable], .textfield-wrapper")) {
      return;
    }
    const touchX = e.touches[0].clientX;
    // Edge trigger zone: within left 32px of the screen
    if (touchX <= 32) {
      startX = touchX;
      startY = e.touches[0].clientY;
      currentX = startX;
      isSwiping = true;
      isHorizontal = null;
      startTime = Date.now();
    }
  }, { passive: true });

  window.addEventListener("touchmove", (e) => {
    if (!isSwiping) return;
    currentX = e.touches[0].clientX;
    const currentY = e.touches[0].clientY;
    const dx = currentX - startX;
    const dy = currentY - startY;

    if (isHorizontal === null && (Math.abs(dx) > 5 || Math.abs(dy) > 5)) {
      isHorizontal = Math.abs(dx) > Math.abs(dy) && dx > 0;
      if (isHorizontal) {
        chatView.classList.add("is-swiping");
        convView.classList.add("is-swiping");
      } else {
        isSwiping = false;
        return;
      }
    }

    if (isHorizontal) {
      const clampedX = Math.max(0, dx);
      chatView.style.transform = `translateX(${clampedX}px)`;

      const progress = Math.min(1, clampedX / window.innerWidth);
      const convOffset = -28 + progress * 28;

      convView.style.transform = `translateX(${convOffset}%)`;
    }
  }, { passive: true });

  const finishEdgeSwipe = () => {
    if (!isSwiping) return;
    const didSwipeHorizontally = isHorizontal === true;
    isSwiping = false;
    isHorizontal = null;
    if (!didSwipeHorizontally) return;

    chatView.classList.remove("is-swiping");
    convView.classList.remove("is-swiping");

    const dx = currentX - startX;
    const elapsed = Date.now() - startTime;
    const velocity = dx / (elapsed || 1);

    if (dx > window.innerWidth * 0.33 || (velocity > 0.38 && dx > 40)) {
      chatView.style.transition = "transform 0.25s cubic-bezier(0.32, 0.72, 0, 1)";
      convView.style.transition = "transform 0.25s cubic-bezier(0.32, 0.72, 0, 1)";
      chatView.style.transform = "translateX(100%)";
      convView.style.transform = "translateX(0)";

      triggerHaptic("light");

      setTimeout(() => {
        chatView.style.transition = "";
        convView.style.transition = "";
        chatView.style.transform = "";
        convView.style.transform = "";
        navigateTo(SubagentManager.backHash());
      }, 250);
    } else {
      chatView.style.transition = "transform 0.22s cubic-bezier(0.32, 0.72, 0, 1)";
      convView.style.transition = "transform 0.22s cubic-bezier(0.32, 0.72, 0, 1)";
      chatView.style.transform = "translateX(0)";
      convView.style.transform = "translateX(-28%)";

      setTimeout(() => {
        chatView.style.transition = "";
        convView.style.transition = "";
        chatView.style.transform = "";
        convView.style.transform = "";
      }, 220);
    }
  };

  window.addEventListener("touchend", finishEdgeSwipe, { passive: true });
  window.addEventListener("touchcancel", finishEdgeSwipe, { passive: true });
}

// --- iOS Virtual Viewport & Keyboard Handling ---

function isKeyboardInputTarget(el) {
  if (!el || el === document.body || el === document.documentElement) return false;
  if (el.isContentEditable) return true;
  const tag = el.tagName;
  if (tag === "TEXTAREA") return !el.readOnly && !el.disabled;
  if (tag === "INPUT") {
    const type = (el.type || "text").toLowerCase();
    const nonTextTypes = new Set([
      "button", "checkbox", "color", "file", "hidden",
      "image", "radio", "range", "reset", "submit"
    ]);
    return !nonTextTypes.has(type) && !el.readOnly && !el.disabled;
  }
  return false;
}

function initVisualViewportHandling() {
  const vp = window.visualViewport;
  if (!vp) return;

  const root = document.documentElement;
  const restingHeights = { portrait: 0, landscape: 0 };
  let kbDismissedAfterBlur = !isKeyboardInputTarget(document.activeElement);
  let blurRecoveryTimer = 0;

  const getOrientationKey = () => {
    if (screen.orientation && typeof screen.orientation.type === "string") {
      return screen.orientation.type.startsWith("landscape") ? "landscape" : "portrait";
    }
    if (typeof window.orientation === "number") {
      return Math.abs(window.orientation) === 90 ? "landscape" : "portrait";
    }
    return window.innerWidth > window.innerHeight ? "landscape" : "portrait";
  };

  const handleViewportChange = () => {
    const orient = getOrientationKey();
    const editableFocused = isKeyboardInputTarget(document.activeElement);

    if (vp.height > restingHeights[orient]) {
      restingHeights[orient] = vp.height;
    }
    if (!editableFocused && window.innerHeight > restingHeights[orient]) {
      restingHeights[orient] = window.innerHeight;
    }

    let baseHeight = Math.max(restingHeights[orient], window.innerHeight);
    if (restingHeights[orient] === 0 && typeof screen !== "undefined" && screen.height > 0) {
      const screenAxis = orient === "portrait"
        ? Math.max(screen.width || 0, screen.height || 0)
        : Math.min(screen.width || 0, screen.height || 0);
      if (screenAxis > baseHeight) baseHeight = screenAxis;
    }

    // iOS 主屏幕独立 PWA 在软键盘弹起时会同时缩小 window.innerHeight 与 visualViewport.height
    // （或伴随 offsetTop 上移），因此必须与未弹键盘时的基准高度比较，而不能用 innerHeight - vp.height - vp.offsetTop。
    const heightDrop = baseHeight - vp.height;
    const kbOpen = !kbDismissedAfterBlur && (heightDrop > 80 || (editableFocused && vp.offsetTop > 20));
    const wasOpen = root.hasAttribute("data-kb-open");

    if (kbOpen) {
      root.setAttribute("data-kb-open", "");
      root.style.setProperty("--vv-height", `${Math.round(vp.height)}px`);
      root.style.setProperty("--vv-offset-top", `${Math.round(Math.max(0, vp.offsetTop))}px`);

      const streamEl = document.getElementById("messages-stream");
      if (streamEl && userIsNearBottom) {
        streamEl.scrollTop = streamEl.scrollHeight;
      }
    } else if (wasOpen) {
      root.removeAttribute("data-kb-open");
      root.style.removeProperty("--vv-height");
      root.style.removeProperty("--vv-offset-top");
      if (typeof syncAppViewport === "function") {
        syncAppViewport();
      }
    }
  };

  vp.addEventListener("resize", handleViewportChange);
  vp.addEventListener("scroll", handleViewportChange);
  window.addEventListener("orientationchange", () => {
    setTimeout(handleViewportChange, 120);
  });

  document.addEventListener("focusin", (e) => {
    if (!isKeyboardInputTarget(e.target)) return;
    kbDismissedAfterBlur = false;
    clearTimeout(blurRecoveryTimer);
    // Avoid synchronous root attribute/style mutations inside focusin while iOS WebKit
    // acquires the assisted input node; let visualViewport events + post-animation checks drive layout.
    setTimeout(handleViewportChange, 100);
    setTimeout(handleViewportChange, 320);
  });

  document.addEventListener("focusout", () => {
    clearTimeout(blurRecoveryTimer);
    // 键盘收起动画完成后兜底清理，规避 iOS Standalone WebKit 偶发收起键盘后 visualViewport.height 卡死不恢复的缺陷
    blurRecoveryTimer = setTimeout(() => {
      if (!isKeyboardInputTarget(document.activeElement)) {
        kbDismissedAfterBlur = true;
        window.scrollTo(0, 0);
        handleViewportChange();
      }
    }, 350);
  });

  handleViewportChange();
}


// ---------------------------------------------------------------------------
// 对话内容全文搜索（language_server SearchConversations，经网关透传）
// 输入停顿 350ms 后再查；少于 2 个字符不查（单字命中太泛）；偏移量按 Unicode 码点计。
// ---------------------------------------------------------------------------
const contentSearch = { timer: 0, seq: 0, results: [], loading: false, query: "" };

function codePointLength(str) {
  let n = 0;
  for (const _ of str) n++;
  return n;
}

// 把片段按码点偏移切成「普通 / 命中」段并转义成 HTML
function highlightSnippetHtml(snippet, ranges) {
  const chars = Array.from(snippet || "");
  const marks = (ranges || [])
    .map((r) => [Math.max(0, r.startOffset | 0), Math.min(chars.length, r.endOffsetExclusive | 0)])
    .filter(([a, b]) => b > a)
    .sort((x, y) => x[0] - y[0]);
  let html = "";
  let cursor = 0;
  for (const [a, b] of marks) {
    if (a < cursor) continue; // 重叠的区间忽略
    html += escapeHtml(chars.slice(cursor, a).join(""));
    html += `<mark>${escapeHtml(chars.slice(a, b).join(""))}</mark>`;
    cursor = b;
  }
  html += escapeHtml(chars.slice(cursor).join(""));
  return html;
}

function syncNoMatchState(contentVisible) {
  document.querySelector("#conversations-list .no-match-state")?.classList.toggle("hidden", !!contentVisible);
}

function renderContentHits() {
  const box = document.getElementById("content-hits");
  if (!box) return;
  const query = (document.getElementById("conv-search")?.value || "").trim();
  if (!query || (!contentSearch.loading && contentSearch.results.length === 0)) {
    box.classList.add("hidden");
    box.innerHTML = "";
    syncNoMatchState(false);
    return;
  }
  // 已经在上面的「标题 / 工作区」匹配里出现的会话不重复展示
  const shown = new Set(
    Object.entries(currentTrajectories || {})
      .filter(([, info]) => !isSubagentSummary(info))
      .filter(([id, info]) => {
        const q = query.toLowerCase();
        const title = formatConversationTitle(info.annotations, info.summary, "").toLowerCase();
        const ws = (info.workspaceUris?.[0] || "").toLowerCase();
        return title.includes(q) || ws.includes(q) || id.includes(q);
      })
      .map(([id]) => id),
  );
  const hits = contentSearch.results.filter((h) => !shown.has(h.cascadeId));
  if (!contentSearch.loading && hits.length === 0) {
    box.classList.add("hidden");
    box.innerHTML = "";
    syncNoMatchState(false);
    return;
  }
  syncNoMatchState(true);
  box.classList.remove("hidden");
  box.innerHTML =
    `<div class="content-hits-header"><span>${typeof t === "function" ? t("对话内容匹配") : "对话内容匹配"}</span>${contentSearch.loading ? '<div class="ios-spinner content-hits-spinner"></div>' : ""}</div>` +
    hits
      .map((h) => {
        const title = h.title && h.title.trim() ? h.title : (typeof t === "function" ? t("未命名会话") : "未命名会话");
        const isSide = Boolean(h.isSideQuestion);
        const parentId = typeof h.parentConversationId === "string" ? h.parentConversationId.trim() : "";
        const metaParts = [];
        if (isSide) metaParts.push("↳ /btw");
        if (h.workspaceName) metaParts.push(h.workspaceName);
        const hitSummary = currentTrajectories?.[h.cascadeId];
        const hitSchedIcon = hitSummary?.isScheduledTask && h.workspaceName && h.workspaceName !== "Chat"
          ? renderScheduledAlarmIconHtml()
          : "";
        const ws = metaParts.length ? `<div class="content-hit-ws">${escapeHtml(metaParts.join(" · "))}${hitSchedIcon}</div>` : "";
        const snippet = h.snippet ? `<div class="content-hit-snippet">${highlightSnippetHtml(h.snippet, h.snippetMatchRanges)}</div>` : "";
        return `<button type="button" class="content-hit-card" data-id="${escapeHtml(h.cascadeId)}" data-parent-id="${escapeHtml(parentId)}" data-is-side-question="${isSide ? "1" : ""}">
          <div class="content-hit-title">${escapeHtml(title)}</div>${snippet}${ws}
        </button>`;
      })
      .join("");
}

function scheduleContentSearch() {
  clearTimeout(contentSearch.timer);
  const query = (document.getElementById("conv-search")?.value || "").trim();
  contentSearch.query = query;
  const seq = ++contentSearch.seq;
  // 当前 Antigravity 没有 SearchConversations 时只保留按标题过滤，不再发必然失败的请求
  if (!isFeatureAvailable(GATEWAY_FEATURE.SEARCH) || codePointLength(query) < 2) {
    contentSearch.results = [];
    contentSearch.loading = false;
    renderContentHits();
    return;
  }
  contentSearch.loading = true;
  renderContentHits();
  contentSearch.timer = setTimeout(async () => {
    try {
      const data = await rpc("SearchConversations", { query });
      if (seq !== contentSearch.seq) return; // 已有更新的查询
      const visible = new Set(
        Object.entries(currentTrajectories || {})
          .filter(([, info]) => !isSubagentSummary(info))
          .map(([id]) => id),
      );
      const results = Array.isArray(data.results) ? data.results : [];
      contentSearch.results = visible.size
        ? results.filter((h) => visible.has(h.cascadeId) || (h.isSideQuestion && h.parentConversationId && visible.has(h.parentConversationId)))
        : results;
    } catch (err) {
      if (seq !== contentSearch.seq) return;
      console.warn("[ContentSearch] failed:", err);
      contentSearch.results = [];
    }
    contentSearch.loading = false;
    renderContentHits();
  }, 350);
}

function initContentSearch() {
  document.getElementById("content-hits")?.addEventListener("click", (e) => {
    const card = e.target.closest(".content-hit-card");
    if (!card) return;
    const id = card.getAttribute("data-id");
    const parentId = (card.getAttribute("data-parent-id") || "").trim();
    const isSide = card.getAttribute("data-is-side-question") === "1";
    if (!id) return;
    if (isSide && parentId && parentId !== id) {
      window._sideQuestionBackParent = window._sideQuestionBackParent || {};
      window._sideQuestionBackParent[id] = parentId;
      try {
        history.pushState(null, "", `#c=${parentId}`);
      } catch {}
    }
    navigateTo(`#c=${id}`);
  });
}


// 聊天页底部悬浮栈的高度会随浮层卡片 / 斜杠列表 / 输入框行数变化，
// 把它同步成 CSS 变量，让消息列表底部留出等高的空白，最后一条消息不会被遮住。
function initBottomStackObserver() {
  const stack = document.getElementById("chat-bottom-stack");
  const view = document.getElementById("view-chat");
  if (!stack || !view || typeof ResizeObserver === "undefined") return;
  const update = () => {
    view.style.setProperty("--bottom-stack-h", Math.ceil(stack.getBoundingClientRect().height) + "px");
  };
  new ResizeObserver(update).observe(stack);
  update();
}

// ==========================================================================
// 新建对话双态启动台（默认速发聚焦 2×2 + 全屏探索配置 2×4，与 iOS / Android 对齐）
// ==========================================================================

window.NewChatLaunchpadPrefs = {
  isEnabled() {
    return localStorage.getItem("agy_new_chat_launchpad") !== "false";
  }
};

const _launchpadDraftGitCache = {};
let _launchpadKeyboardFocused = false;
let _launchpadListenersBound = false;
let _launchpadShuffleSeed = Math.floor(Math.random() * 999999) + 1;
let _launchpadLastDraftKey = "";
let _launchpadDesktopExploreMode = false;

const _LP_LOCAL_SLASH_KEY = "mgy_local_slash_usage_v1";
const _LP_SHORTCUTS_CACHE_KEY = "mgy_launchpad_shortcuts_v1";
const _LP_MAX_SLOTS = 4;
const _LP_MID_CHAT_EXCLUDED = new Set(["compact", "btw", "schedule", "browser"]);

function _loadLocalSlashUsage() {
  try {
    const raw = localStorage.getItem(_LP_LOCAL_SLASH_KEY);
    const parsed = raw ? JSON.parse(raw) : {};
    return parsed && typeof parsed === "object" ? parsed : {};
  } catch {
    return {};
  }
}

window.recordLocalSlashUsage = function (name) {
  const clean = String(name || "").trim().replace(/^\/+/, "").toLowerCase();
  if (!clean) return;
  try {
    const map = _loadLocalSlashUsage();
    map[clean] = (Number(map[clean]) || 0) + 1;
    localStorage.setItem(_LP_LOCAL_SLASH_KEY, JSON.stringify(map));
  } catch {}
};

function _normalizeShortcutItem(item) {
  if (!item || typeof item !== "object") return null;
  const kind = String(item.kind || "").trim().toLowerCase();
  if (!["workflow", "slash", "skill", "prompt"].includes(kind)) return null;
  let ref = String(item.ref || "").trim();
  if (kind === "slash" || kind === "skill") {
    ref = ref.replace(/^\/+/, "").toLowerCase();
  }
  if (!ref) return null;
  return {
    kind,
    ref,
    title: item.title ? String(item.title).trim() : "",
    subtitle: item.subtitle ? String(item.subtitle).trim() : "",
    svgKey: item.svgKey ? String(item.svgKey).trim() : "",
    tint: item.tint ? String(item.tint).trim() : ""
  };
}

function _shortcutStableId(item) {
  return `${String(item?.kind || "").toLowerCase()}:${String(item?.ref || "").replace(/^\/+/, "").toLowerCase()}`;
}

window.LaunchpadShortcutsStore = (() => {
  let state = { customized: false, items: [] };
  let loaded = false;
  let fetchedOnce = false;

  function loadLocal() {
    if (loaded) return state;
    loaded = true;
    try {
      const raw = localStorage.getItem(_LP_SHORTCUTS_CACHE_KEY);
      if (raw) {
        const parsed = JSON.parse(raw);
        const list = Array.isArray(parsed?.items)
          ? parsed.items.map(_normalizeShortcutItem).filter(Boolean).slice(0, _LP_MAX_SLOTS)
          : [];
        const custom = !!parsed?.customized && list.length > 0;
        state = { customized: custom, items: custom ? list : [] };
      }
    } catch {}
    return state;
  }

  function saveLocal(next) {
    state = next;
    try {
      localStorage.setItem(_LP_SHORTCUTS_CACHE_KEY, JSON.stringify(next));
    } catch {}
  }

  function isPinned(kind, ref) {
    loadLocal();
    const id = _shortcutStableId({ kind, ref });
    return state.items.some((x) => _shortcutStableId(x) === id);
  }

  async function refresh(force = false) {
    loadLocal();
    if (fetchedOnce && !force) return state;
    fetchedOnce = true;
    try {
      const resp = await fetch("/gateway/launchpad-shortcuts");
      if (!resp.ok) return state;
      const data = await resp.json();
      const list = Array.isArray(data?.items)
        ? data.items.map(_normalizeShortcutItem).filter(Boolean).slice(0, _LP_MAX_SLOTS)
        : [];
      const custom = !!data?.customized && list.length > 0;
      const next = { customized: custom, items: custom ? list : [] };
      if (JSON.stringify(next) !== JSON.stringify(state)) {
        saveLocal(next);
        window.refreshNewChatLaunchpad?.();
      }
    } catch {}
    return state;
  }

  async function applyAndSync(rawList, customized) {
    loadLocal();
    const dedup = [];
    const seen = new Set();
    for (const raw of rawList || []) {
      const norm = _normalizeShortcutItem(raw);
      if (!norm) continue;
      const id = _shortcutStableId(norm);
      if (seen.has(id)) continue;
      seen.add(id);
      dedup.push(norm);
      if (dedup.length >= _LP_MAX_SLOTS) break;
    }
    const effectiveCustom = !!customized && dedup.length > 0;
    const next = { customized: effectiveCustom, items: effectiveCustom ? dedup : [] };
    saveLocal(next);
    window.refreshNewChatLaunchpad?.();

    try {
      const resp = effectiveCustom
        ? await fetch("/gateway/launchpad-shortcuts", {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ customized: true, items: next.items })
          })
        : await fetch("/gateway/launchpad-shortcuts", { method: "DELETE" });
      if (resp.ok) {
        const data = await resp.json();
        const serverList = Array.isArray(data?.items)
          ? data.items.map(_normalizeShortcutItem).filter(Boolean).slice(0, _LP_MAX_SLOTS)
          : [];
        const serverCustom = !!data?.customized && serverList.length > 0;
        const synced = { customized: serverCustom, items: serverCustom ? serverList : [] };
        if (JSON.stringify(synced) !== JSON.stringify(state)) {
          saveLocal(synced);
          window.refreshNewChatLaunchpad?.();
        }
      }
    } catch {}
    return state;
  }

  function pin(item) {
    loadLocal();
    const norm = _normalizeShortcutItem(item);
    if (!norm) return { status: "invalid" };
    if (isPinned(norm.kind, norm.ref)) return { status: "already" };
    if (state.items.length >= _LP_MAX_SLOTS) return { status: "full" };
    const next = state.items.concat([norm]);
    applyAndSync(next, true);
    return { status: "added", count: next.length };
  }

  function unpin(kind, ref) {
    loadLocal();
    const id = _shortcutStableId({ kind, ref });
    const next = state.items.filter((x) => _shortcutStableId(x) !== id);
    applyAndSync(next, next.length > 0);
  }

  function removeAt(idx) {
    loadLocal();
    if (idx < 0 || idx >= state.items.length) return;
    const next = state.items.slice();
    next.splice(idx, 1);
    applyAndSync(next, next.length > 0);
  }

  function replaceAt(idx, item) {
    loadLocal();
    const norm = _normalizeShortcutItem(item);
    if (!norm || idx < 0 || idx >= state.items.length) return;
    const id = _shortcutStableId(norm);
    const next = state.items.filter((x) => _shortcutStableId(x) !== id);
    const safeIdx = Math.min(idx, next.length);
    if (next.length >= _LP_MAX_SLOTS && safeIdx < next.length) {
      next[safeIdx] = norm;
    } else if (next.length < _LP_MAX_SLOTS) {
      next.splice(safeIdx, 0, norm);
    }
    applyAndSync(next.slice(0, _LP_MAX_SLOTS), next.length > 0);
  }

  function move(fromIdx, toIdx) {
    loadLocal();
    if (fromIdx < 0 || fromIdx >= state.items.length || toIdx < 0 || toIdx >= state.items.length || fromIdx === toIdx) return;
    const next = state.items.slice();
    const [moved] = next.splice(fromIdx, 1);
    next.splice(toIdx, 0, moved);
    applyAndSync(next, true);
  }

  function reset() {
    applyAndSync([], false);
  }

  return {
    getState: () => loadLocal(),
    isPinned,
    refresh,
    pin,
    unpin,
    removeAt,
    replaceAt,
    move,
    reset
  };
})();

// 启动台专属 24×24 矢量 SVG 图标目录（与 iOS / Android 像素对齐，全程严禁洋红）
const _LP_SVG_PATHS = {
  slash_plan: "M 8 4.5 L 16 4.5 C 17.1 4.5 18 5.4 18 6.5 L 18 19 C 18 20.1 17.1 21 16 21 L 8 21 C 6.9 21 6 20.1 6 19 L 6 6.5 C 6 5.4 6.9 4.5 8 4.5 Z M 9.5 3 L 14.5 3 L 14.5 6 L 9.5 6 Z M 9 11 L 10.6 12.6 L 13.5 9.7 M 9 16 L 15 16",
  slash_grill: "M 4 6.5 C 4 5.1 5.1 4 6.5 4 L 14.5 4 C 15.9 4 17 5.1 17 6.5 L 17 11.5 C 17 12.9 15.9 14 14.5 14 L 9 14 L 5.5 16.8 L 5.5 14 C 4.6 13.5 4 12.6 4 11.5 Z M 17 9.5 L 18 9.5 C 19.1 9.5 20 10.4 20 11.5 L 20 16 C 20 17.1 19.1 18 18 18 L 17.5 18 L 17.5 20.5 L 14.3 18 L 11 18",
  slash_boost: "M 13.2 2.5 L 5.2 13 L 11.6 13 L 10.4 21.5 L 18.8 11 L 12.4 11 Z",
  slash_goal: "M 12 3.5 C 16.7 3.5 20.5 7.3 20.5 12 C 20.5 16.7 16.7 20.5 12 20.5 C 7.3 20.5 3.5 16.7 3.5 12 C 3.5 7.3 7.3 3.5 12 3.5 Z M 12 7.5 C 14.5 7.5 16.5 9.5 16.5 12 C 16.5 14.5 14.5 16.5 12 16.5 C 9.5 16.5 7.5 14.5 7.5 12 C 7.5 9.5 9.5 7.5 12 7.5 Z M 12 10.8 C 12.7 10.8 13.2 11.3 13.2 12 C 13.2 12.7 12.7 13.2 12 13.2 C 11.3 13.2 10.8 12.7 10.8 12 C 10.8 11.3 11.3 10.8 12 10.8 Z",
  slash_teamwork: "M 12 3.5 C 13.6 3.5 14.8 4.7 14.8 6.3 C 14.8 7.9 13.6 9.1 12 9.1 C 10.4 9.1 9.2 7.9 9.2 6.3 C 9.2 4.7 10.4 3.5 12 3.5 Z M 5.8 15 C 7.2 15 8.3 16.1 8.3 17.5 C 8.3 18.9 7.2 20 5.8 20 C 4.4 20 3.3 18.9 3.3 17.5 C 3.3 16.1 4.4 15 5.8 15 Z M 18.2 15 C 19.6 15 20.7 16.1 20.7 17.5 C 20.7 18.9 19.6 20 18.2 20 C 16.8 20 15.7 18.9 15.7 17.5 C 15.7 16.1 16.8 15 18.2 15 Z M 10.4 8.8 L 7.2 15.2 M 13.6 8.8 L 16.8 15.2 M 8.3 17.5 L 15.7 17.5",
  slash_swarm: "M 12 3 L 16.2 5.4 L 16.2 10.2 L 12 12.6 L 7.8 10.2 L 7.8 5.4 Z M 7.8 10.2 L 12 12.6 L 12 17.4 L 7.8 19.8 L 3.6 17.4 L 3.6 12.6 Z M 16.2 10.2 L 20.4 12.6 L 20.4 17.4 L 16.2 19.8 L 12 17.4 L 12 12.6 Z",
  slash_workflow: "M 4 5.5 C 4 4.7 4.7 4 5.5 4 L 9.5 4 C 10.3 4 11 4.7 11 5.5 L 11 8.5 C 11 9.3 10.3 10 9.5 10 L 5.5 10 C 4.7 10 4 9.3 4 8.5 Z M 13 15.5 C 13 14.7 13.7 14 14.5 14 L 18.5 14 C 19.3 14 20 14.7 20 15.5 L 20 18.5 C 20 19.3 19.3 20 18.5 20 L 14.5 20 C 13.7 20 13 19.3 13 18.5 Z M 11 7 L 15.5 7 C 16.3 7 16.5 7.5 16.5 8.5 L 16.5 14 M 7.5 10 L 7.5 15.5 C 7.5 16.5 8.2 17 9 17 L 13 17",
  slash_learn: "M 9 17.5 L 15 17.5 M 9.8 20.5 L 14.2 20.5 M 9.5 14.5 C 7.2 13.2 6 11 6 8.8 C 6 5.5 8.7 3 12 3 C 15.3 3 18 5.5 18 8.8 C 18 11 16.8 13.2 14.5 14.5 L 14.5 15.5 L 9.5 15.5 Z",
  slash_automation: "M 12 4 C 16.4 4 20 7.6 20 12 C 20 16.4 16.4 20 12 20 C 7.6 20 4 16.4 4 12 C 4 9.4 5.2 7.1 7.2 5.6 M 4.2 5.2 L 7.6 5.2 L 7.6 8.6 M 12 7.5 L 12 12 L 15.2 13.8",
  slash_skill: "M 6.2 7.2 L 9.2 7.2 C 7.6 3.4 14.0 3.4 12.4 7.2 L 15.4 7.2 C 16.2 7.2 16.8 7.8 16.8 8.6 L 16.8 11.6 C 20.6 10.0 20.6 16.4 16.8 14.8 L 16.8 17.8 C 16.8 18.6 16.2 19.2 15.4 19.2 L 6.2 19.2 C 5.4 19.2 4.8 18.6 4.8 17.8 L 4.8 8.6 C 4.8 7.8 5.4 7.2 6.2 7.2 Z",
  custom_pin: "M 9 4 L 15 4 L 14.2 10 L 17.2 12.8 L 17.2 14.5 L 6.8 14.5 L 6.8 12.8 L 9.8 10 Z M 12 14.5 L 12 20.5",
  shuffle_refresh: "M 4 7.5 L 8.2 7.5 C 10.2 7.5 11.4 9.2 12.4 11.2 L 13.2 12.8 C 14.2 14.8 15.4 16.5 17.4 16.5 L 20 16.5 M 17.5 14 L 20 16.5 L 17.5 19 M 4 16.5 L 8.2 16.5 C 9.6 16.5 10.6 15.6 11.4 14.3 M 13.8 9.7 C 14.6 8.4 15.6 7.5 17.4 7.5 L 20 7.5 M 17.5 5 L 20 7.5 L 17.5 10",
  prompt_fact_check: "M 12 3.5 C 16.7 3.5 20.5 7.3 20.5 12 C 20.5 16.7 16.7 20.5 12 20.5 C 7.3 20.5 3.5 16.7 3.5 12 C 3.5 7.3 7.3 3.5 12 3.5 Z M 3.8 12 L 20.2 12 M 12 3.5 C 14.4 6 15.6 8.8 15.6 12 C 15.6 15.2 14.4 18 12 20.5 C 9.6 18 8.4 15.2 8.4 12 C 8.4 8.8 9.6 6 12 3.5 Z",
  prompt_draft: "M 15.8 4.2 L 19.8 8.2 L 9.2 18.8 L 4.5 19.5 L 5.2 14.8 Z M 13.4 6.6 L 17.4 10.6 M 13.5 19.5 L 20 19.5",
  prompt_compare: "M 12 4 L 12 19.5 M 8 19.5 L 16 19.5 M 5 7.5 L 19 7.5 M 6.2 7.5 L 3.8 13.2 C 4.5 14.4 7.9 14.4 8.6 13.2 Z M 17.8 7.5 L 15.4 13.2 C 16.1 14.4 19.5 14.4 20.2 13.2 Z",
  prompt_digest: "M 5.5 3.5 L 14.5 3.5 L 18.5 7.5 L 18.5 20.5 L 5.5 20.5 Z M 14.5 3.5 L 14.5 7.5 L 18.5 7.5 M 8.5 11.5 L 15.5 11.5 M 8.5 15 L 15.5 15 M 8.5 18 L 12.2 18",
  prompt_research: "M 12 3.5 C 16.7 3.5 20.5 7.3 20.5 12 C 20.5 16.7 16.7 20.5 12 20.5 C 7.3 20.5 3.5 16.7 3.5 12 C 3.5 7.3 7.3 3.5 12 3.5 Z M 15.6 8.4 L 13.6 13.6 L 8.4 15.6 L 10.4 10.4 Z",
  prompt_brainstorm: "M 4.5 6.5 L 6.3 8.3 L 9.5 5.1 M 12 7 L 19.5 7 M 4.5 12 L 6.3 13.8 L 9.5 10.6 M 12 12.5 L 19.5 12.5 M 4.5 17.5 L 6.3 19.3 L 9.5 16.1 M 12 18 L 17 18",
  prompt_translate: "M 4 6.5 L 11.5 6.5 M 7.8 4.2 L 7.8 6.5 M 9.8 6.5 C 9.2 9.8 7.2 12.4 4.5 14 M 6.2 9.2 C 7.5 11 9.3 12.6 11.2 13.6 M 13.2 19.5 L 16.4 11 L 19.6 19.5 M 14.3 16.8 L 18.5 16.8",
  prompt_notes: "M 4.5 5.5 L 19.5 5.5 L 19.5 9.5 L 4.5 9.5 Z M 6.5 9.5 L 6.5 18.5 C 6.5 19.3 7.2 20 8 20 L 16 20 C 16.8 20 17.5 19.3 17.5 18.5 L 17.5 9.5 M 9.5 13.5 L 14.5 13.5 M 9.5 16.5 L 13 16.5",
  prompt_git_diff: "M 10.5 4.2 C 14 4.2 16.8 7 16.8 10.5 C 16.8 14 14 16.8 10.5 16.8 C 7 16.8 4.2 14 4.2 10.5 C 4.2 7 7 4.2 10.5 4.2 Z M 15.2 15.2 L 20 20 M 8.3 10.5 L 12.7 10.5 M 10.5 8.3 L 10.5 12.7",
  prompt_arch: "M 12 3.8 L 20 8 L 12 12.2 L 4 8 Z M 4 12.2 L 12 16.4 L 20 12.2 M 4 16.2 L 12 20.4 L 20 16.2",
  prompt_bug: "M 9 7.5 L 9 6.5 C 9 4.8 10.3 3.5 12 3.5 C 13.7 3.5 15 4.8 15 6.5 L 15 7.5 M 8.5 7.5 L 15.5 7.5 C 16.9 7.5 18 8.6 18 10 L 18 14 C 18 17.3 15.3 20 12 20 C 8.7 20 6 17.3 6 14 L 6 10 C 6 8.6 7.1 7.5 8.5 7.5 Z M 12 10.8 L 12 20 M 6 10.5 L 3 9 M 6 14 L 2.8 14 M 18 10.5 L 21 9 M 18 14 L 21.2 14",
  prompt_refactor: "M 4.5 19.5 L 15.5 8.5 M 14.2 6.8 L 17.2 9.8 M 12 3.2 L 12 5.5 M 18.5 12 L 20.8 12 M 17.5 4.5 L 19 3 M 18.5 6.5 L 19.8 5.2 M 7 5 L 7.7 7.1 L 9.8 7.8 L 7.7 8.5 L 7 10.6 L 6.3 8.5 L 4.2 7.8 L 6.3 7.1 Z",
  prompt_tests: "M 9.2 3.5 L 14.8 3.5 M 10.2 3.5 L 10.2 9.2 L 5.4 17.8 C 4.8 18.9 5.6 20.2 6.8 20.2 L 17.2 20.2 C 18.4 20.2 19.2 18.9 18.6 17.8 L 13.8 9.2 L 13.8 3.5 M 7.6 15.2 L 16.4 15.2",
  prompt_git_log: "M 7 4 C 8.4 4 9.5 5.1 9.5 6.5 C 9.5 7.9 8.4 9 7 9 C 5.6 9 4.5 7.9 4.5 6.5 C 4.5 5.1 5.6 4 7 4 Z M 7 15 C 8.4 15 9.5 16.1 9.5 17.5 C 9.5 18.9 8.4 20 7 20 C 5.6 20 4.5 18.9 4.5 17.5 C 4.5 16.1 5.6 15 7 15 Z M 17.5 7 C 18.9 7 20 8.1 20 9.5 C 20 10.9 18.9 12 17.5 12 C 16.1 12 15 10.9 15 9.5 C 15 8.1 16.1 7 17.5 7 Z M 7 9 L 7 15 M 17.5 12 C 17.5 15.2 14.2 17.5 9.5 17.5",
  prompt_security: "M 12 3 L 19 6 L 19 11.8 C 19 16.4 15.9 19.7 12 21 C 8.1 19.7 5 16.4 5 11.8 L 5 6 Z M 9.2 12.2 L 11.2 14.2 L 15.2 10",
  prompt_changelog: "M 4.5 12.4 L 12.4 4.5 L 19.5 4.5 L 19.5 11.6 L 11.6 19.5 Z M 15.2 8.2 C 15.8 8.2 16.3 8.7 16.3 9.3 C 16.3 9.9 15.8 10.4 15.2 10.4 C 14.6 10.4 14.1 9.9 14.1 9.3 C 14.1 8.7 14.6 8.2 15.2 8.2 Z"
};

function _renderLaunchpadSvg(key, size = 14) {
  let d = null;
  if (typeof key === "string" && key.startsWith("wf:") && typeof WORKFLOW_ICON_CATALOG !== "undefined") {
    const wfKey = typeof normalizeWorkflowIconKey === "function" ? normalizeWorkflowIconKey(key.slice(3)) : key.slice(3);
    const hit = WORKFLOW_ICON_CATALOG.find((ic) => ic.key === wfKey) || WORKFLOW_ICON_CATALOG[0];
    d = hit?.path;
  }
  if (!d) d = _LP_SVG_PATHS[key] || _LP_SVG_PATHS.slash_skill;
  return `<svg width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.95" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="${d}"></path></svg>`;
}

const _LP_CORE_SLASH_POOL = [
  { name: "plan", svg: "slash_plan", tintName: "blue", tint: "#3b82f6", zh: "/plan 方案规划", en: "/plan Architect", ja: "/plan 実装計画", sZh: "先出步骤与风险，确认再动手", sEn: "Align steps & risks before editing", sJa: "手順とリスクを固めてから着手", fallbackIcon: "ballot" },
  { name: "grill-me", svg: "slash_grill", tintName: "indigo", tint: "#6366f1", zh: "/grill-me 追问对齐", en: "/grill-me Clarify", ja: "/grill-me 深掘り", sZh: "逐轮反向提问，拆清模糊想法", sEn: "Interactive Q&A to sharpen ideas", sJa: "逆質問で曖昧な要件を明確化", fallbackIcon: "forum" },
  { name: "boost", svg: "slash_boost", tintName: "amber", tint: "#f59e0b", zh: "/boost 深度攻坚", en: "/boost Deep Think", ja: "/boost 徹底推論", sZh: "多视角深度推理与严格验证", sEn: "Deep multi-angle reasoning & checks", sJa: "多角的な深い推論と厳格検証", fallbackIcon: "bolt" },
  { name: "goal", svg: "slash_goal", tintName: "emerald", tint: "#22c55e", zh: "/goal 自律闭环", en: "/goal Autonomous", ja: "/goal 自律達成", sZh: "持续迭代自检，达标才收工", sEn: "Iterate & self-verify until done", sJa: "目標達成まで自律検証を継続", fallbackIcon: "timer" },
  { name: "teamwork", svg: "slash_teamwork", tintName: "cyan", tint: "#06b6d4", zh: "/teamwork 团队协作", en: "/teamwork Agents", ja: "/teamwork 連携", sZh: "多角色子 Agent 分工推进大任务", sEn: "Coordinate specialist subagents", sJa: "複数エージェントで分担推進", fallbackIcon: "graph_2" },
  { name: "swarm", svg: "slash_swarm", tintName: "amber", tint: "#f59e0b", zh: "/swarm 蜂群并发", en: "/swarm Parallel", ja: "/swarm 並列群", sZh: "共享消息板多 Agent 协同攻坚", sEn: "Shared board for parallel workers", sJa: "共有ボードで並列エージェント実行", fallbackIcon: "hub" },
  { name: "workflow", svg: "slash_workflow", tintName: "teal", tint: "#14b8a6", zh: "/workflow 编排流程", en: "/workflow Pipeline", ja: "/workflow 編成", sZh: "并行或流水线批量执行多阶段任务", sEn: "Orchestrate staged agent pipelines", sJa: "多段階ワークフローを一括編成", fallbackIcon: "graph_2" },
  { name: "learn", svg: "slash_learn", tintName: "amber", tint: "#f59e0b", zh: "/learn 沉淀经验", en: "/learn Persist", ja: "/learn 知見定着", sZh: "将踩坑与规范固化，后续自动生效", sEn: "Save lessons & rules for future chats", sJa: "学びやルールを今後の会話に定着", fallbackIcon: "lightbulb_2" },
  { name: "automation", svg: "slash_automation", tintName: "blue", tint: "#3b82f6", zh: "/automation 定时任务", en: "/automation Cron", ja: "/automation 定期", sZh: "创建周期巡检或定时提醒任务", sEn: "Create recurring background schedules", sJa: "定期巡回やリマインドを自動化", fallbackIcon: "schedule" }
];

const _LP_PURE_PROMPT_POOL = [
  { id: "pure_fact_check", svg: "prompt_fact_check", tintName: "blue", tint: "#3b82f6", zh: "联网核查资讯", en: "Web Fact-Check", ja: "最新情報を検証", sZh: "交叉验证最新进展与来源", sEn: "Verify latest updates & sources", sJa: "信頼できる情報源で事実確認", pZh: "请帮我联网核查以下消息的最新进展，并附上权威来源交叉印证：", pEn: "Please verify the latest updates and cross-check reliable sources for: ", pJa: "以下の件の最新状況と信頼できる情報源を突き合わせて確認してください：" },
  { id: "pure_draft", svg: "prompt_draft", tintName: "teal", tint: "#14b8a6", zh: "起草与润色", en: "Draft & Polish", ja: "下書き・推敲", sZh: "文章 / 公告 / 邮件 / 提纲", sEn: "Articles, notes, emails & outlines", sJa: "記事・お知らせ・メール・構成案", pZh: "帮我起草并润色一段清晰自然的内容，主题是：", pEn: "Help me draft and polish a clear, natural piece about: ", pJa: "次のテーマについて自然で分かりやすい文章を作成・推敲してください：" },
  { id: "pure_compare", svg: "prompt_compare", tintName: "amber", tint: "#f59e0b", zh: "多维方案对比", en: "Compare Options", ja: "多角的に比較", sZh: "权衡利弊、成本与风险", sEn: "Weigh pros, cons, cost & risk", sJa: "利点・コスト・リスクを比較整理", pZh: "请从利弊、成本与风险角度对比以下方案，并给出建议：", pEn: "Please compare these options across pros, cons, cost, and risk, then recommend one: ", pJa: "以下の案をメリット・コスト・リスクの観点で比較し、推奨案を示してください：" },
  { id: "pure_digest", svg: "prompt_digest", tintName: "indigo", tint: "#6366f1", zh: "长文精读提炼", en: "Distill Long Text", ja: "長文の要点抽出", sZh: "抓核心结论与关键数据", sEn: "Extract key takeaways & figures", sJa: "核心結論と重要データを整理", pZh: "请帮我精读下面内容，提炼核心结论、关键数据与潜在漏洞：", pEn: "Please read the following closely and extract key conclusions, data, and blind spots: ", pJa: "以下の内容を精読し、核心結論・主要データ・見落としを抽出してください：" },
  { id: "pure_research", svg: "prompt_research", tintName: "cyan", tint: "#06b6d4", zh: "深度专题调研", en: "Topic Deep Dive", ja: "テーマ徹底調査", sZh: "梳理来龙去脉与现状趋势", sEn: "Map background, status & trends", sJa: "背景・現状・今後の動向を整理", pZh: "请帮我做一次系统的专题调研，梳理背景、现状与关键趋势：", pEn: "Please run a structured deep dive covering background, current state, and key trends on: ", pJa: "次のテーマについて背景・現状・主要トレンドを体系的に調査してください：" },
  { id: "pure_brainstorm", svg: "prompt_brainstorm", tintName: "emerald", tint: "#22c55e", zh: "拆解模糊想法", en: "Shape Rough Ideas", ja: "アイデア具体化", sZh: "把灵感变成可执行清单", sEn: "Turn sparks into action checklists", sJa: "着想を実行可能な手順へ分解", pZh: "我有一个初步想法，请帮我拆解关键问题并整理成行动清单：", pEn: "I have a rough idea—help me break down the key questions into an actionable checklist: ", pJa: "まだ曖昧なアイデアがあります。論点を分解し実行リストに整理してください：" },
  { id: "pure_translate", svg: "prompt_translate", tintName: "blue", tint: "#3b82f6", zh: "地道翻译润色", en: "Natural Translation", ja: "自然な翻訳推敲", sZh: "保留原意与语境的自然表达", sEn: "Idiomatic phrasing with exact nuance", sJa: "ニュアンスを保った自然な表現", pZh: "请将下面内容地道地翻译并润色，保持自然专业语气：", pEn: "Please translate and polish the following naturally while preserving nuance and tone: ", pJa: "以下の内容を元の意図と専門的なトーンを保って自然に翻訳・推敲してください：" },
  { id: "pure_notes", svg: "prompt_notes", tintName: "teal", tint: "#14b8a6", zh: "零散记录整理", en: "Organize Notes", ja: "メモを構造化", sZh: "转成结构化纪要与待办", sEn: "Structure minutes, decisions & TODOs", sJa: "議事録・決定事項・TODO化", pZh: "请把以下零散记录整理成结构化纪要，列出结论与下一步待办：", pEn: "Please turn these rough notes into structured minutes with decisions and next actions: ", pJa: "以下の断片メモを構造化し、結論と次のアクション一覧にまとめてください：" }
];

const _LP_WS_PROMPT_POOL = [
  { id: "ws_git_diff", svg: "prompt_git_diff", tintName: "amber", tint: "#f59e0b", requiresDirty: true, zh: "审查未提交改动", en: "Review Dirty Diff", ja: "未コミット変更確認", sZh: "检查 git diff 风险与优化", sEn: "Inspect git diff risks & fixes", sJa: "git diff のリスクと改善確認", pZh: "请先检查当前未提交的改动，指出潜在风险并给出优化建议", pEn: "Please review the current uncommitted changes, highlight potential risks, and suggest improvements", pJa: "現在の未コミット変更を確認し、潜在リスクと改善案を教えてください" },
  { id: "ws_arch", svg: "prompt_arch", tintName: "indigo", tint: "#6366f1", zh: "梳理项目架构", en: "Map Architecture", ja: "構成を把握", sZh: "快速定位核心模块与入口", sEn: "Find core modules & entry points", sJa: "主要モジュールと入口を整理", pZh: "请先梳理这个工作区的整体架构、核心模块分工与关键入口文件", pEn: "Please map out the overall architecture, core modules, and key entry points of this workspace", pJa: "このワークスペースの全体構造、主要モジュール、エントリポイントを整理してください" },
  { id: "ws_bug", svg: "prompt_bug", tintName: "coral", tint: "#ef4444", zh: "定位排查 Bug", en: "Debug Root Cause", ja: "バグ原因調査", sZh: "结合代码与日志追查根因", sEn: "Trace root cause from code & logs", sJa: "コードとログから根本原因特定", pZh: "请帮我排查并定位以下问题的根因，先分析再给修复方案：", pEn: "Please investigate and find the root cause of this issue before proposing a fix: ", pJa: "以下の問題の根本原因を分析し、修正案を提示してください：" },
  { id: "ws_refactor", svg: "prompt_refactor", tintName: "teal", tint: "#14b8a6", zh: "代码优化重构", en: "Refactor & Polish", ja: "最適化・改善", sZh: "提升可读性、性能与稳定性", sEn: "Improve clarity, speed & stability", sJa: "可読性・性能・安定性を改善", pZh: "请分析当前模块可优化的设计点，给出轻量低侵入的重构方案：", pEn: "Please identify improvements in the current module and propose a low-impact refactor: ", pJa: "現在のモジュールの改善点を分析し、低侵襲なリファクタ案を提案してください：" },
  { id: "ws_tests", svg: "prompt_tests", tintName: "emerald", tint: "#22c55e", zh: "补全单元测试", en: "Add Unit Tests", ja: "単体テスト追加", sZh: "覆盖核心路径与边界条件", sEn: "Cover core flows & edge cases", sJa: "主要経路と境界ケースを網羅", pZh: "请为以下模块补充单元测试，重点覆盖异常分支与边界情况：", pEn: "Please add high-value unit tests focusing on edge cases and error paths for: ", pJa: "以下のモジュールに異常系・境界条件を重視した単体テストを追加してください：" },
  { id: "ws_git_log", svg: "prompt_git_log", tintName: "cyan", tint: "#06b6d4", zh: "速览近期提交", en: "Summarize Commits", ja: "直近コミット要約", sZh: "总结最近变更与影响范围", sEn: "Recap recent git log & blast radius", sJa: "最近の変更点と影響範囲を把握", pZh: "请结合最近的 git log 梳理近期核心改动，并评估影响范围：", pEn: "Please summarize recent key commits from git log and assess their impact area: ", pJa: "最近の git log から主な変更点を要約し、影響範囲を評価してください：" },
  { id: "ws_security", svg: "prompt_security", tintName: "amber", tint: "#f59e0b", zh: "安全隐患体检", en: "Security Checkup", ja: "セキュリティ点検", sZh: "排查权限、输入与异常兜底", sEn: "Audit auth, inputs & error handling", sJa: "権限・入力検証・例外処理を確認", pZh: "请对当前工作区关键链路做一次轻量安全与健壮性检查：", pEn: "Please perform a lightweight security and resilience audit on the critical paths here: ", pJa: "このワークスペースの重要経路について軽量なセキュリティ・堅牢性チェックを行ってください：" },
  { id: "ws_changelog", svg: "prompt_changelog", tintName: "blue", tint: "#3b82f6", zh: "生成变更说明", en: "Draft Changelog", ja: "変更履歴を作成", sZh: "整理发版日志或 PR 描述", sEn: "Write release notes or PR summary", sJa: "リリースノートや PR 概要を整形", pZh: "请基于当前分支近期改动，整理一份简洁清晰的变更说明：", pEn: "Please draft clear, concise release notes or a PR summary from recent branch changes: ", pJa: "現在のブランチの直近変更から簡潔な変更履歴・PR説明を作成してください：" }
];

const _LP_ALL_PROMPTS = _LP_PURE_PROMPT_POOL.concat(_LP_WS_PROMPT_POOL);

function _lpTintColor(colorKey) {
  switch (String(colorKey || "").toLowerCase()) {
    case "indigo":
    case "purple": return "#5e5ce6";
    case "green":
    case "emerald": return "#30d158";
    case "blue": return "#0a84ff";
    case "teal": return "#30b0c7";
    case "cyan": return "#06b6d4";
    case "red":
    case "coral": return "#ff453a";
    case "orange":
    case "amber": return "#ff9f0a";
    default: return "#5e5ce6";
  }
}

function _lpSafeTintName(colorKey) {
  switch (String(colorKey || "").toLowerCase()) {
    case "indigo":
    case "purple": return "indigo";
    case "green":
    case "emerald": return "green";
    case "blue": return "blue";
    case "teal": return "teal";
    case "cyan": return "cyan";
    case "red":
    case "coral": return "red";
    case "orange":
    case "amber": return "orange";
    default: return "indigo";
  }
}

function _lpDeterministicShuffle(items, seed) {
  if (!Array.isArray(items) || items.length <= 1) return items.slice();
  const arr = items.slice();
  let state = ((seed || 1) >>> 0) || 1;
  for (let i = arr.length - 1; i > 0; i--) {
    state = (Math.imul(state, 1664525) + 1013904223) >>> 0;
    const j = state % (i + 1);
    const tmp = arr[i];
    arr[i] = arr[j];
    arr[j] = tmp;
  }
  return arr;
}

function _isMobileTouchLayout() {
  return window.innerWidth <= 820;
}

function _bindLaunchpadKeyboardListeners() {
  if (_launchpadListenersBound) return;
  _launchpadListenersBound = true;
  const inputEl = document.getElementById("chat-input");
  if (!inputEl) return;
  inputEl.addEventListener("focus", () => {
    if (_isMobileTouchLayout()) {
      _launchpadKeyboardFocused = true;
      window.refreshNewChatLaunchpad?.();
    }
  });
  inputEl.addEventListener("blur", () => {
    if (_launchpadKeyboardFocused) {
      _launchpadKeyboardFocused = false;
      window.refreshNewChatLaunchpad?.();
    }
  });
}

function _showLaunchpadToast(message) {
  let el = document.getElementById("lp-toast-banner");
  if (!el) {
    el = document.createElement("div");
    el.id = "lp-toast-banner";
    el.className = "lp-toast-banner";
    document.body.appendChild(el);
  }
  el.textContent = message;
  el.classList.add("visible");
  clearTimeout(el.__hideTimer);
  el.__hideTimer = setTimeout(() => {
    el.classList.remove("visible");
  }, 1900);
}

function _buildLaunchpadData(isPure, dirtyCount) {
  const isEnLang = typeof isEn === "function" && isEn();
  const isJaLang = typeof isJa === "function" && isJa();
  const pick3 = (zh, en, ja) => (isEnLang ? en : (isJaLang ? ja : zh));

  const gatewayCommands = (typeof slashState !== "undefined" && Array.isArray(slashState.commands)) ? slashState.commands : [];
  const gatewayByName = new Map();
  gatewayCommands.forEach((c) => {
    if (c && c.name) gatewayByName.set(String(c.name).toLowerCase(), c);
  });
  const coreByName = new Map(_LP_CORE_SLASH_POOL.map((s) => [s.name, s]));
  const templates = (typeof workflowState !== "undefined" && Array.isArray(workflowState.templates))
    ? workflowState.templates
    : (typeof loadCachedWorkflowTemplates === "function" ? loadCachedWorkflowTemplates() : []);

  const makeCoreCard = (spec) => {
    const live = gatewayByName.get(spec.name);
    const title = pick3(spec.zh, spec.en, spec.ja);
    const sub = pick3(spec.sZh, spec.sEn, spec.sJa);
    return {
      svg: spec.svg,
      tint: spec.tint,
      title,
      sub,
      slashName: spec.name,
      slashFallback: live || {
        name: spec.name, title, description: sub, icon: spec.fallbackIcon, kind: "system", definitionPath: ""
      },
      shortcutItem: { kind: "slash", ref: spec.name, title, subtitle: sub, svgKey: spec.svg, tint: spec.tintName }
    };
  };

  const makeDynamicSlashCard = (opt) => {
    const lower = String(opt.name || "").toLowerCase();
    const spec = coreByName.get(lower);
    if (spec) return makeCoreCard(spec);
    const desc = String(opt.description || "").trim();
    const kind = String(opt.kind || "").toLowerCase() === "system" ? "slash" : "skill";
    return {
      svg: "slash_skill",
      tint: "#14b8a6",
      title: `/${opt.name}`,
      sub: desc || pick3("高频常用技能指令", "Frequently used skill", "よく使うスキルコマンド"),
      slashName: opt.name,
      slashFallback: opt,
      shortcutItem: { kind, ref: opt.name, title: `/${opt.name}`, subtitle: desc, svgKey: "slash_skill", tint: "teal" }
    };
  };

  const makePromptCard = (spec) => {
    const title = (spec.id === "ws_git_diff" && dirtyCount > 0)
      ? pick3(`审查 ${dirtyCount} 个改动`, `Review ${dirtyCount} changes`, `変更 ${dirtyCount} 件を確認`)
      : pick3(spec.zh, spec.en, spec.ja);
    const sub = pick3(spec.sZh, spec.sEn, spec.sJa);
    return {
      svg: spec.svg,
      tint: spec.tint,
      title,
      sub,
      prompt: pick3(spec.pZh, spec.pEn, spec.pJa),
      shortcutItem: { kind: "prompt", ref: spec.id, title: pick3(spec.zh, spec.en, spec.ja), subtitle: sub, svgKey: spec.svg, tint: spec.tintName }
    };
  };

  // 状态一：2×2 快捷起手（未自定义时不套用自定义 Workflow：上排 2 随机系统核心斜杠 + 下排 2 按模式随机模板）
  const shortcutState = window.LaunchpadShortcutsStore.getState();
  const focusCards = [];
  const usedSlashNames = new Set();
  const usedPromptIds = new Set();

  if (shortcutState.customized) {
    for (const item of shortcutState.items.slice(0, _LP_MAX_SLOTS)) {
      if (item.kind === "workflow") {
        const tpl = templates.find((x) => x && x.id === item.ref);
        if (!tpl) continue;
        const wfIconKey = `wf:${typeof normalizeWorkflowIconKey === "function" ? normalizeWorkflowIconKey(tpl.icon) : (tpl.icon || "workflow")}`;
        focusCards.push({
          svg: wfIconKey,
          tint: _lpTintColor(tpl.color),
          title: tpl.title || pick3("自定义快捷模板", "Custom quick template", "カスタムショートカット"),
          sub: tpl.subtitle || pick3("自定义快捷模板", "Custom quick template", "カスタムショートカット"),
          prompt: tpl.templatePrompt || "",
          shortcutItem: item
        });
      } else if (item.kind === "slash" || item.kind === "skill") {
        const lower = item.ref.toLowerCase();
        usedSlashNames.add(lower);
        const core = coreByName.get(lower);
        const live = gatewayByName.get(lower);
        if (core) {
          focusCards.push(makeCoreCard(core));
        } else if (live) {
          focusCards.push(makeDynamicSlashCard(live));
        } else {
          focusCards.push({
            svg: item.svgKey || "slash_skill",
            tint: _lpTintColor(item.tint || "teal"),
            title: item.title || `/${lower}`,
            sub: item.subtitle || pick3("自定义斜杠指令", "Custom slash command", "カスタムコマンド"),
            slashName: lower,
            slashFallback: { name: lower, title: item.title || `/${lower}`, description: item.subtitle || "", icon: "terminal", kind: item.kind === "slash" ? "system" : "skill" },
            shortcutItem: item
          });
        }
      } else if (item.kind === "prompt") {
        const spec = _LP_ALL_PROMPTS.find((p) => p.id === item.ref);
        if (!spec) continue;
        usedPromptIds.add(spec.id);
        focusCards.push(makePromptCard(spec));
      }
    }
  }

  // 上排补齐（最多补至 2 格）：基于纯对话或代码工作区的随机提示词模板
  const promptPool = isPure
    ? _LP_PURE_PROMPT_POOL
    : _LP_WS_PROMPT_POOL.filter((item) => !item.requiresDirty || dirtyCount > 0);
  if (focusCards.length < 2) {
    const filteredPrompts = promptPool.filter((p) => !usedPromptIds.has(p.id));
    for (const spec of _lpDeterministicShuffle(filteredPrompts, _launchpadShuffleSeed ^ 0x7E4B21)) {
      if (focusCards.length >= 2) break;
      usedPromptIds.add(spec.id);
      focusCards.push(makePromptCard(spec));
    }
  }

  // 下排补齐（补满至 4 格）：随机系统核心斜杠（用户自选 2 项时，下排正好为 2 个随机斜杠）
  const intersectedCore = gatewayCommands.length > 0
    ? _LP_CORE_SLASH_POOL.filter((s) => gatewayByName.has(s.name))
    : _LP_CORE_SLASH_POOL;
  if (focusCards.length < 4) {
    const coreAvail = intersectedCore.length >= 2 ? intersectedCore : _LP_CORE_SLASH_POOL;
    const slashPool = coreAvail.filter((s) => !usedSlashNames.has(s.name));
    const finalSlashPool = slashPool.length >= 2 ? slashPool : _LP_CORE_SLASH_POOL.filter((s) => !usedSlashNames.has(s.name));
    for (const spec of _lpDeterministicShuffle(finalSlashPool, _launchpadShuffleSeed ^ 0x13579B)) {
      if (focusCards.length >= 4) break;
      usedSlashNames.add(spec.name);
      focusCards.push(makeCoreCard(spec));
    }
  }

  // 状态二：2×4 学习区（上 4 随机核心斜杠 + 下 4 按模式切换的提示词池）
  const availableCore = intersectedCore.length >= 4 ? intersectedCore : _LP_CORE_SLASH_POOL;
  const exploreSlashCards = _lpDeterministicShuffle(availableCore, _launchpadShuffleSeed ^ 0x5A3C91)
    .slice(0, 4)
    .map(makeCoreCard);

  const explorePromptCards = _lpDeterministicShuffle(promptPool, _launchpadShuffleSeed ^ 0x2F8B4D)
    .slice(0, 4)
    .map(makePromptCard);

  return { focusCards: focusCards.slice(0, 4), exploreSlashCards, explorePromptCards, shortcutState };
}

function _openLaunchpadConfiguratorModal(pendingItem = null) {
  const isEnLang = typeof isEn === "function" && isEn();
  const isJaLang = typeof isJa === "function" && isJa();
  const pick3 = (zh, en, ja) => (isEnLang ? en : (isJaLang ? ja : zh));

  let overlay = document.getElementById("lp-config-modal");
  if (!overlay) {
    overlay = document.createElement("div");
    overlay.id = "lp-config-modal";
    overlay.className = "lp-config-overlay hidden";
    document.body.appendChild(overlay);
    overlay.addEventListener("click", (e) => {
      if (e.target === overlay) overlay.classList.add("hidden");
    });
  }

  let activeTab = "workflow";
  let searchQuery = "";
  let activePending = pendingItem ? _normalizeShortcutItem(pendingItem) : null;

  const gatewayCommands = (typeof slashState !== "undefined" && Array.isArray(slashState.commands)) ? slashState.commands : [];
  const gatewayByName = new Map();
  gatewayCommands.forEach((c) => {
    if (c && c.name) gatewayByName.set(String(c.name).toLowerCase(), c);
  });
  const templates = (typeof workflowState !== "undefined" && Array.isArray(workflowState.templates))
    ? workflowState.templates
    : (typeof loadCachedWorkflowTemplates === "function" ? loadCachedWorkflowTemplates() : []);
  const localCounts = _loadLocalSlashUsage();

  function resolveDisplay(item) {
    const norm = _normalizeShortcutItem(item) || item;
    if (norm.kind === "workflow") {
      const wf = templates.find((x) => x && x.id === norm.ref);
      if (wf) {
        const wfIconKey = `wf:${typeof normalizeWorkflowIconKey === "function" ? normalizeWorkflowIconKey(wf.icon) : (wf.icon || "workflow")}`;
        return { svg: wfIconKey, tint: _lpTintColor(wf.color), title: wf.title, sub: wf.subtitle || "Workflow" };
      }
      const fallbackIcon = String(norm.svgKey || "").startsWith("wf:") ? norm.svgKey : "wf:workflow";
      return { svg: fallbackIcon, tint: _lpTintColor(norm.tint || "indigo"), title: norm.title || norm.ref, sub: norm.subtitle || "Workflow" };
    }
    if (norm.kind === "slash" || norm.kind === "skill") {
      const core = _LP_CORE_SLASH_POOL.find((s) => s.name === norm.ref);
      if (core) return { svg: core.svg, tint: core.tint, title: pick3(core.zh, core.en, core.ja), sub: pick3(core.sZh, core.sEn, core.sJa) };
      const live = gatewayByName.get(norm.ref);
      if (live) return { svg: "slash_skill", tint: "#14b8a6", title: `/${live.name}`, sub: live.description || pick3("已安装技能", "Installed skill", "スキル") };
      return { svg: norm.svgKey || "slash_skill", tint: _lpTintColor(norm.tint || "teal"), title: norm.title || `/${norm.ref}`, sub: norm.subtitle || "" };
    }
    const spec = _LP_ALL_PROMPTS.find((p) => p.id === norm.ref);
    if (spec) return { svg: spec.svg, tint: spec.tint, title: pick3(spec.zh, spec.en, spec.ja), sub: pick3(spec.sZh, spec.sEn, spec.sJa) };
    return { svg: norm.svgKey || "prompt_draft", tint: _lpTintColor(norm.tint || "blue"), title: norm.title || norm.ref, sub: norm.subtitle || "" };
  }

  function buildCandidates() {
    const q = searchQuery.trim().toLowerCase();
    if (activeTab === "workflow") {
      return templates
        .filter((tpl) => tpl && (!q || String(tpl.title || "").toLowerCase().includes(q) || String(tpl.subtitle || "").toLowerCase().includes(q)))
        .map((tpl) => {
          const wfIconKey = `wf:${typeof normalizeWorkflowIconKey === "function" ? normalizeWorkflowIconKey(tpl.icon) : (tpl.icon || "workflow")}`;
          return {
            item: { kind: "workflow", ref: tpl.id, title: tpl.title, subtitle: tpl.subtitle, svgKey: wfIconKey, tint: _lpSafeTintName(tpl.color) },
            svg: wfIconKey,
            tint: _lpTintColor(tpl.color),
            title: tpl.title,
            sub: tpl.subtitle || tpl.templatePrompt || "",
            badge: tpl.inQuickMenu ? pick3("快选", "Quick", "クイック") : ""
          };
        });
    }
    if (activeTab === "slash") {
      const coreNames = new Set(_LP_CORE_SLASH_POOL.map((s) => s.name));
      const list = _LP_CORE_SLASH_POOL
        .filter((s) => !q || s.name.includes(q) || s.zh.toLowerCase().includes(q) || s.en.toLowerCase().includes(q) || s.sZh.toLowerCase().includes(q))
        .map((s) => {
          const count = Math.max(Number(gatewayByName.get(s.name)?.usageCount) || 0, Number(localCounts[s.name]) || 0);
          const title = pick3(s.zh, s.en, s.ja);
          const sub = pick3(s.sZh, s.sEn, s.sJa);
          return {
            item: { kind: "slash", ref: s.name, title, subtitle: sub, svgKey: s.svg, tint: s.tintName },
            svg: s.svg,
            tint: s.tint,
            title,
            sub,
            badge: count > 0 ? pick3(`用过 ${count} 次`, `${count}×`, `${count}回`) : ""
          };
        });
      gatewayCommands.forEach((cmd) => {
        const lower = String(cmd?.name || "").toLowerCase();
        if (String(cmd?.kind || "").toLowerCase() !== "system" || coreNames.has(lower) || _LP_MID_CHAT_EXCLUDED.has(lower)) return;
        if (q && !lower.includes(q) && !String(cmd.description || "").toLowerCase().includes(q)) return;
        const count = Math.max(Number(cmd.usageCount) || 0, Number(localCounts[lower]) || 0);
        list.push({
          item: { kind: "slash", ref: lower, title: `/${cmd.name}`, subtitle: cmd.description || "", svgKey: "slash_skill", tint: "blue" },
          svg: "slash_skill",
          tint: "#3b82f6",
          title: `/${cmd.name}`,
          sub: cmd.description || pick3("系统斜杠指令", "System slash command", "システムコマンド"),
          badge: count > 0 ? pick3(`用过 ${count} 次`, `${count}×`, `${count}回`) : ""
        });
      });
      return list;
    }
    if (activeTab === "skill") {
      const slashExcludedNames = new Set(_LP_CORE_SLASH_POOL.map((s) => s.name));
      _LP_MID_CHAT_EXCLUDED.forEach((n) => slashExcludedNames.add(n));
      return gatewayCommands
        .filter((cmd) => {
          const lower = String(cmd?.name || "").toLowerCase();
          return cmd && lower && String(cmd.kind || "").toLowerCase() !== "system" && !slashExcludedNames.has(lower);
        })
        .filter((cmd) => !q || String(cmd.name || "").toLowerCase().includes(q) || String(cmd.description || "").toLowerCase().includes(q))
        .sort((a, b) => {
          const ca = Math.max(Number(a.usageCount) || 0, Number(localCounts[String(a.name || "").toLowerCase()]) || 0);
          const cb = Math.max(Number(b.usageCount) || 0, Number(localCounts[String(b.name || "").toLowerCase()]) || 0);
          return (cb - ca) || String(a.name).localeCompare(String(b.name));
        })
        .map((cmd) => {
          const lower = String(cmd.name || "").toLowerCase();
          const count = Math.max(Number(cmd.usageCount) || 0, Number(localCounts[lower]) || 0);
          return {
            item: { kind: "skill", ref: lower, title: `/${cmd.name}`, subtitle: cmd.description || "", svgKey: "slash_skill", tint: "teal" },
            svg: "slash_skill",
            tint: "#14b8a6",
            title: `/${cmd.name}`,
            sub: cmd.description || pick3("已安装技能", "Installed skill", "スキル"),
            badge: count > 0 ? pick3(`用过 ${count} 次`, `${count}×`, `${count}回`) : ""
          };
        });
    }
    return _LP_ALL_PROMPTS
      .filter((p) => !q || p.zh.toLowerCase().includes(q) || p.en.toLowerCase().includes(q) || p.sZh.toLowerCase().includes(q))
      .map((p) => {
        const title = pick3(p.zh, p.en, p.ja);
        const sub = pick3(p.sZh, p.sEn, p.sJa);
        return {
          item: { kind: "prompt", ref: p.id, title, subtitle: sub, svgKey: p.svg, tint: p.tintName },
          svg: p.svg,
          tint: p.tint,
          title,
          sub,
          badge: p.id.startsWith("pure_") ? pick3("纯对话", "Chat", "チャット") : pick3("代码区", "Code", "コード")
        };
      });
  }

  function renderModal() {
    const st = window.LaunchpadShortcutsStore.getState();
    const candidates = buildCandidates();
    const isSwap = !!activePending;
    const pendingInfo = activePending ? resolveDisplay(activePending) : null;

    overlay.innerHTML = `
      <div class="lp-config-sheet">
        <div class="lp-config-header">
          <div class="lp-config-title">${escapeHtml(pick3("自定义 2×2 快捷指令", "Customize 2×2 Shortcuts", "2×2 ショートカット設定"))}</div>
          <button type="button" class="lp-config-done" data-lpc="close">${escapeHtml(pick3("完成", "Done", "完了"))}</button>
        </div>
        <div class="lp-config-body">
          ${pendingInfo ? `
            <div class="lp-config-swap-banner">
              <span class="launchpad-card-icon-badge" style="color:${pendingInfo.tint};background:${pendingInfo.tint}22">${_renderLaunchpadSvg(pendingInfo.svg, 14)}</span>
              <div class="lp-swap-text">
                <div class="lp-swap-sub">${escapeHtml(pick3("2×2 已满，点击下方槽位替换为：", "2×2 full — click a slot below to replace with:", "満枠です。下の枠をクリックして置換："))}</div>
                <div class="lp-swap-name">${escapeHtml(pendingInfo.title)}</div>
              </div>
              <button type="button" class="lp-swap-cancel" data-lpc="cancel-swap">${escapeHtml(pick3("取消", "Cancel", "取消"))}</button>
            </div>
          ` : ""}

          <div class="lp-config-section-head">
            <span>${escapeHtml(pick3(`当前已固定 (${st.items.length}/4)`, `Pinned Slots (${st.items.length}/4)`, `固定済み (${st.items.length}/4)`))}</span>
            ${st.customized ? `<button type="button" class="lp-config-reset" data-lpc="reset">${escapeHtml(pick3("恢复智能推荐", "Smart Default", "おすすめに戻す"))}</button>` : ""}
          </div>

          <div class="lp-slots-grid">
            ${[0, 1, 2, 3].map((idx) => {
              const item = st.items[idx];
              if (!item) {
                return `<div class="lp-slot-empty"><span>+</span><span>${escapeHtml(pick3(`空位 ${idx + 1} · 自动补位`, `Slot ${idx + 1} · Auto`, `枠 ${idx + 1} · 自動`))}</span></div>`;
              }
              const info = resolveDisplay(item);
              return `
                <div class="lp-slot-card ${isSwap ? "swap-target" : ""}" data-lpc-slot="${idx}">
                  <div class="lp-slot-top">
                    <span class="launchpad-card-icon-badge" style="color:${info.tint};background:${info.tint}22">${_renderLaunchpadSvg(info.svg, 12.5)}</span>
                    <span class="lp-slot-title">${escapeHtml(info.title)}</span>
                    ${!isSwap ? `<button type="button" class="lp-slot-del" data-lpc-del="${idx}" aria-label="Remove">×</button>` : ""}
                  </div>
                  <div class="lp-slot-bottom">
                    <span class="lp-slot-sub ${isSwap ? "amber" : ""}">${escapeHtml(isSwap ? pick3("点此替换", "Click to replace", "クリックで置換") : info.sub)}</span>
                    ${(!isSwap && st.items.length > 1) ? `
                      <span class="lp-slot-arrows">
                        ${idx > 0 ? `<button type="button" class="lp-slot-move" data-lpc-move="${idx}:${idx - 1}">‹</button>` : ""}
                        ${idx < st.items.length - 1 ? `<button type="button" class="lp-slot-move" data-lpc-move="${idx}:${idx + 1}">›</button>` : ""}
                      </span>
                    ` : ""}
                  </div>
                </div>
              `;
            }).join("")}
          </div>

          <div class="lp-config-tabs">
            ${[
              ["workflow", "Workflow"],
              ["slash", pick3("斜杠", "Slash", "スラッシュ")],
              ["skill", pick3("技能", "Skills", "スキル")],
              ["prompt", pick3("模板", "Prompts", "テンプレート")]
            ].map(([k, lbl]) => `
              <button type="button" class="lp-config-tab ${activeTab === k ? "active" : ""}" data-lpc-tab="${k}">${escapeHtml(lbl)}</button>
            `).join("")}
          </div>

          <input type="text" class="lp-config-search" id="lp-config-search-input" value="${escapeHtml(searchQuery)}" placeholder="${escapeHtml(pick3("搜索指令、技能或模板...", "Search commands, skills or prompts...", "検索..."))}" />

          <div class="lp-candidate-list">
            ${candidates.map((c, i) => {
              const pinned = window.LaunchpadShortcutsStore.isPinned(c.item.kind, c.item.ref);
              return `
                <button type="button" class="lp-candidate-row ${pinned ? "pinned" : ""}" data-lpc-cand="${i}">
                  <span class="launchpad-card-icon-badge lg" style="color:${c.tint};background:${c.tint}22">${_renderLaunchpadSvg(c.svg, 15)}</span>
                  <span class="lp-candidate-info">
                    <span class="lp-candidate-head">
                      <span class="lp-candidate-title">${escapeHtml(c.title)}</span>
                      ${c.badge ? `<span class="lp-candidate-badge">${escapeHtml(c.badge)}</span>` : ""}
                    </span>
                    <span class="lp-candidate-sub">${escapeHtml(c.sub)}</span>
                  </span>
                  <span class="lp-candidate-check">${pinned ? "✓" : "+"}</span>
                </button>
              `;
            }).join("")}
            ${activeTab === "workflow" ? `
              <button type="button" class="lp-manage-wf-btn" data-lpc="open-wf">${escapeHtml(pick3("新建或编辑 Workflow 模板", "Create or edit Workflow templates", "Workflow テンプレートを管理"))}</button>
            ` : ""}
          </div>
        </div>
      </div>
    `;

    overlay.querySelector('[data-lpc="close"]')?.addEventListener("click", () => overlay.classList.add("hidden"));
    overlay.querySelector('[data-lpc="cancel-swap"]')?.addEventListener("click", () => {
      activePending = null;
      renderModal();
    });
    overlay.querySelector('[data-lpc="reset"]')?.addEventListener("click", () => {
      window.LaunchpadShortcutsStore.reset();
      _showLaunchpadToast(pick3("已恢复智能推荐", "Reset to smart defaults", "おすすめに戻しました"));
      renderModal();
    });
    overlay.querySelector('[data-lpc="open-wf"]')?.addEventListener("click", () => {
      overlay.classList.add("hidden");
      if (typeof openWorkflowSheet === "function") openWorkflowSheet(false);
    });
    overlay.querySelectorAll("[data-lpc-del]").forEach((btn) => {
      btn.addEventListener("click", (e) => {
        e.stopPropagation();
        window.LaunchpadShortcutsStore.removeAt(parseInt(btn.getAttribute("data-lpc-del"), 10));
        renderModal();
      });
    });
    overlay.querySelectorAll("[data-lpc-move]").forEach((btn) => {
      btn.addEventListener("click", (e) => {
        e.stopPropagation();
        const [f, tIdx] = String(btn.getAttribute("data-lpc-move")).split(":").map(Number);
        window.LaunchpadShortcutsStore.move(f, tIdx);
        renderModal();
      });
    });
    if (isSwap) {
      overlay.querySelectorAll("[data-lpc-slot]").forEach((slotEl) => {
        slotEl.addEventListener("click", () => {
          const idx = parseInt(slotEl.getAttribute("data-lpc-slot"), 10);
          window.LaunchpadShortcutsStore.replaceAt(idx, activePending);
          activePending = null;
          _showLaunchpadToast(pick3(`已替换槽位 ${idx + 1}`, `Replaced slot ${idx + 1}`, `枠 ${idx + 1} を置換しました`));
          renderModal();
        });
      });
    }
    overlay.querySelectorAll("[data-lpc-tab]").forEach((btn) => {
      btn.addEventListener("click", () => {
        activeTab = btn.getAttribute("data-lpc-tab");
        renderModal();
      });
    });
    const searchEl = overlay.querySelector("#lp-config-search-input");
    if (searchEl) {
      searchEl.addEventListener("input", () => {
        searchQuery = searchEl.value;
        const pos = searchEl.selectionStart;
        renderModal();
        const nextInput = overlay.querySelector("#lp-config-search-input");
        if (nextInput) {
          nextInput.focus();
          if (typeof pos === "number") nextInput.setSelectionRange(pos, pos);
        }
      });
    }
    overlay.querySelectorAll("[data-lpc-cand]").forEach((btn) => {
      btn.addEventListener("click", () => {
        const c = candidates[parseInt(btn.getAttribute("data-lpc-cand"), 10)];
        if (!c) return;
        if (window.LaunchpadShortcutsStore.isPinned(c.item.kind, c.item.ref)) {
          window.LaunchpadShortcutsStore.unpin(c.item.kind, c.item.ref);
          renderModal();
          return;
        }
        const res = window.LaunchpadShortcutsStore.pin(c.item);
        if (res.status === "added") {
          _showLaunchpadToast(pick3(`已固定到 2×2 (${res.count}/4)`, `Pinned to 2×2 (${res.count}/4)`, `2×2 に固定 (${res.count}/4)`));
          renderModal();
        } else if (res.status === "full") {
          _showLaunchpadToast(pick3("已选满 4 项，请先移除一项", "4/4 slots full — remove one first", "4枠満枠です。1つ削除してください"));
        }
      });
    });
  }

  overlay.classList.remove("hidden");
  window.LaunchpadShortcutsStore.refresh(true).then(() => renderModal());
  renderModal();
}

function _fetchDraftGitStatusIfNeeded(uri) {
  if (!uri || _launchpadDraftGitCache[uri]?.pending) return;
  const cached = _launchpadDraftGitCache[uri];
  if (cached && Date.now() - cached.fetchedAt < 15000) return;
  _launchpadDraftGitCache[uri] = { ...(cached || {}), pending: true };
  const fetcher = typeof postGatewayJson === "function"
    ? postGatewayJson("/gateway/git/status", { cascadeId: "", workspaceUri: uri })
    : fetch("/gateway/git/status", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ cascadeId: "", workspaceUri: uri })
      }).then((r) => (r.ok ? r.json() : null));
  Promise.resolve(fetcher)
    .then((st) => {
      _launchpadDraftGitCache[uri] = {
        branch: st && st.branch ? String(st.branch).trim() : "",
        dirtyCount: st && Array.isArray(st.files) ? st.files.length : 0,
        fetchedAt: Date.now(),
        pending: false
      };
      window.refreshNewChatLaunchpad?.();
    })
    .catch(() => {
      _launchpadDraftGitCache[uri] = { branch: "", dirtyCount: 0, fetchedAt: Date.now(), pending: false };
    });
}

window.renderNewChatLaunchpad = function (streamEl, draft) {
  if (!streamEl || !draft) return;
  _bindLaunchpadKeyboardListeners();
  const isPure = !!draft.isPure;
  const enabled = window.NewChatLaunchpadPrefs.isEnabled();

  const draftKey = `${draft.isPure ? "pure" : (draft.uri || draft.name || "")}`;
  if (draftKey !== _launchpadLastDraftKey) {
    _launchpadLastDraftKey = draftKey;
    _launchpadShuffleSeed = Math.floor(Math.random() * 999999) + 1;
    _launchpadDesktopExploreMode = false;
  }

  if (!enabled) {
    streamEl.innerHTML = `
      <div class="chat-empty-state">
        <div class="chat-empty-icon">
          <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
            <path d="m12 3-1.912 5.813a2 2 0 0 1-1.275 1.275L3 12l5.813 1.912a2 2 0 0 1 1.275 1.275L12 21l1.912-5.813a2 2 0 0 1 1.275-1.275L21 12l-5.813-1.912a2 2 0 0 1-1.275-1.275L12 3Z"></path>
          </svg>
        </div>
        <div class="chat-empty-title">${isPure ? (typeof t === "function" ? t("新对话") : "新对话") : escapeHtml(draft.name)}</div>
        <div class="chat-empty-desc">${isPure ? (typeof t === "function" ? t("新对话模式，在下方输入指令开启对话") : "新对话模式，在下方输入指令开启对话") : (typeof t === "function" ? t("已连接工作区，在下方输入指令开启对话") : "已连接工作区，在下方输入指令开启对话")}</div>
      </div>
    `;
    return;
  }

  if (typeof ensureSlashCommands === "function") {
    try { ensureSlashCommands(); } catch {}
  }
  window.LaunchpadShortcutsStore.refresh(false);
  if (!isPure && draft.uri) {
    _fetchDraftGitStatusIfNeeded(draft.uri);
  }

  const gitInfo = (!isPure && draft.uri) ? (_launchpadDraftGitCache[draft.uri] || {}) : {};
  const gitBranch = gitInfo.branch || "";
  const dirtyCount = gitInfo.dirtyCount || 0;
  const isMobile = _isMobileTouchLayout();
  const isFocusState = isMobile ? _launchpadKeyboardFocused : !_launchpadDesktopExploreMode;

  const isEnLang = typeof isEn === "function" && isEn();
  const isJaLang = typeof isJa === "function" && isJa();
  const pick3 = (zh, en, ja) => (isEnLang ? en : (isJaLang ? ja : zh));

  const switchLabel = pick3("切换", "Switch", "切替");
  const pickWsLabel = pick3("选工作区", "Workspace", "選択");
  const cleanLabel = pick3("干净", "Clean", "クリーン");
  const pureDesc = pick3("纯对话模式 · 不关联代码工作区", "Pure chat · No workspace linked", "チャット専用 · ワークスペース未連携");
  const dirtyLabel = pick3(`${dirtyCount} 改动`, `${dirtyCount} dirty`, `${dirtyCount} 変更`);

  const { focusCards, exploreSlashCards, explorePromptCards, shortcutState } = _buildLaunchpadData(isPure, dirtyCount);

  const wsBarHtml = `
    <div class="launchpad-ws-bar">
      ${isPure ? `
        <span class="lp-ws-icon">💬</span>
        <span class="lp-ws-pure">${escapeHtml(pureDesc)}</span>
        <button type="button" class="lp-ws-switch" data-lp-action="switch-ws">${escapeHtml(pickWsLabel)}</button>
      ` : `
        <span class="lp-ws-icon">📁</span>
        <span class="lp-ws-name">${escapeHtml(draft.name || "")}</span>
        ${gitBranch ? `<span class="lp-ws-branch">${escapeHtml(gitBranch)}</span>` : ""}
        ${dirtyCount > 0
          ? `<button type="button" class="lp-ws-dirty" data-lp-action="dirty">${escapeHtml(dirtyLabel)}</button>`
          : (gitBranch ? `<span class="lp-ws-clean">${escapeHtml(cleanLabel)}</span>` : "")}
        <span class="lp-spacer"></span>
        <button type="button" class="lp-ws-switch" data-lp-action="switch-ws">${escapeHtml(switchLabel)}</button>
      `}
    </div>
  `;

  const renderCardHtml = (c, poolKey, idx, allowPinBadge) => {
    const pinned = allowPinBadge && c.shortcutItem && window.LaunchpadShortcutsStore.isPinned(c.shortcutItem.kind, c.shortcutItem.ref);
    return `
      <button type="button" class="launchpad-card ${pinned ? "is-pinned" : ""}" data-lp-pool="${poolKey}" data-lp-idx="${idx}" title="${allowPinBadge ? escapeHtml(pick3("单击试用 · 右键或长按固定到 2×2", "Click to use · Right-click or long-press to pin", "クリックで使用・長押し/右クリックで固定")) : ""}">
        <div class="launchpad-card-title">
          <span class="launchpad-card-icon-badge" style="color:${c.tint};background:${c.tint}21">${_renderLaunchpadSvg(c.svg, 13.5)}</span>
          <span class="launchpad-card-title-text">${escapeHtml(c.title)}</span>
          ${pinned ? `<span class="lp-card-pin-dot" aria-label="Pinned">📌</span>` : ""}
        </div>
        <div class="launchpad-card-sub">${escapeHtml(c.sub)}</div>
      </button>
    `;
  };

  let bodyHtml = "";
  if (isFocusState) {
    bodyHtml = `
      <div class="launchpad-body launchpad-body-focus">
        <div class="launchpad-section">
          <div class="launchpad-section-head">
            <div class="launchpad-section-title">${escapeHtml(pick3("快捷起手", "Quick Start", "クイック起動"))}</div>
            <div class="launchpad-head-actions">
              ${!isMobile ? `<button type="button" class="launchpad-mode-pill" data-lp-action="toggle-mode">${escapeHtml(pick3("探索与配置", "Explore & Edit", "探索と設定"))}</button>` : `
                <button type="button" class="launchpad-explore-pill" data-lp-action="enter-explore">
                  <span>${escapeHtml(pick3("探索与配置 ›", "Explore & Edit ›", "探索と設定 ›"))}</span>
                </button>
              `}
            </div>
          </div>
          <div class="launchpad-grid">
            ${focusCards.map((c, idx) => renderCardHtml(c, "focus", idx, false)).join("")}
          </div>
        </div>
      </div>
    `;
  } else {
    const configSub = shortcutState.customized
      ? pick3(`已固定 ${shortcutState.items.length}/4 项 · 右键/长按下方卡片亦可一键添加`, `${shortcutState.items.length}/4 pinned · Right-click/long-press cards below to pin`, `${shortcutState.items.length}/4 固定済み · 下のカード長押しで追加`)
      : pick3("当前为随机推荐 · 右键/长按下方卡片可一键固定", "Random discovery · Right-click/long-press any card below to pin", "ランダム表示中 · カード長押しで固定可能");

    bodyHtml = `
      <div class="launchpad-body launchpad-body-explore">
        <div class="launchpad-config-row">
          <button type="button" class="launchpad-config-entry" data-lp-action="open-config">
            <span class="lp-cfg-icon">⊞</span>
            <span class="lp-cfg-text">
              <span class="lp-cfg-title">${escapeHtml(pick3("自定义 2×2 快捷指令", "Customize 2×2 Shortcuts", "2×2 ショートカット設定"))}</span>
              <span class="lp-cfg-sub">${escapeHtml(configSub)}</span>
            </span>
            <span class="lp-chevron">›</span>
          </button>
          ${!isMobile ? `<button type="button" class="launchpad-mode-pill" data-lp-action="toggle-mode">${escapeHtml(pick3("快捷 2×2", "Quick 2×2", "2×2"))}</button>` : ""}
        </div>

        <div class="launchpad-section">
          <div class="launchpad-section-head">
            <div class="launchpad-section-title">${escapeHtml(pick3("探索斜杠指令", "Discover Slash Commands", "スラッシュコマンド探索"))}</div>
            <button type="button" class="launchpad-shuffle-btn" data-lp-action="shuffle">
              ${_renderLaunchpadSvg("shuffle_refresh", 13)}
              <span>${escapeHtml(pick3("换一批", "Shuffle", "入れ替え"))}</span>
            </button>
          </div>
          <div class="launchpad-grid">
            ${exploreSlashCards.map((c, idx) => renderCardHtml(c, "slash", idx, true)).join("")}
          </div>
        </div>

        <div class="launchpad-section">
          <div class="launchpad-section-head">
            <div class="launchpad-section-title">${escapeHtml(isPure ? pick3("纯对话任务表达", "Chat Prompt Ideas", "チャット向けプロンプト例") : pick3("代码工作区任务表达", "Workspace Prompt Ideas", "開発ワークスペース向け例"))}</div>
          </div>
          <div class="launchpad-grid">
            ${explorePromptCards.map((c, idx) => renderCardHtml(c, "prompt", idx, true)).join("")}
          </div>
        </div>
      </div>
    `;
  }

  streamEl.innerHTML = `
    <div class="launchpad-root" data-lp-blank="${isFocusState ? "1" : "0"}">
      ${wsBarHtml}
      ${bodyHtml}
    </div>
  `;

  const pools = { focus: focusCards, slash: exploreSlashCards, prompt: explorePromptCards };

  const triggerCard = (card) => {
    if (!card) return;
    if (card.slashName) {
      window.recordLocalSlashUsage?.(card.slashName);
      if (typeof selectSlashCommand === "function") {
        selectSlashCommand(card.slashName, card.slashFallback);
      }
      return;
    }
    const inputEl = document.getElementById("chat-input");
    if (inputEl) {
      inputEl.value = card.prompt || "";
      inputEl.style.height = "auto";
      inputEl.style.height = Math.min(inputEl.scrollHeight, 120) + "px";
      inputEl.focus();
      if (typeof inputEl.setSelectionRange === "function") {
        const openIdx = (card.prompt || "").indexOf("[");
        const closeIdx = openIdx >= 0 ? (card.prompt || "").indexOf("]", openIdx) : -1;
        if (openIdx >= 0 && closeIdx > openIdx) {
          inputEl.setSelectionRange(openIdx, closeIdx + 1);
        } else {
          const len = (card.prompt || "").length;
          inputEl.setSelectionRange(len, len);
        }
      }
    }
  };

  const handlePinAction = (card) => {
    if (!card || !card.shortcutItem) return;
    const item = card.shortcutItem;
    if (window.LaunchpadShortcutsStore.isPinned(item.kind, item.ref)) {
      if (typeof triggerHaptic === "function") triggerHaptic("light");
      window.LaunchpadShortcutsStore.unpin(item.kind, item.ref);
      _showLaunchpadToast(pick3("已从 2×2 移除", "Removed from 2×2", "2×2 から削除しました"));
      return;
    }
    const res = window.LaunchpadShortcutsStore.pin(item);
    if (res.status === "added") {
      if (typeof triggerHaptic === "function") triggerHaptic("medium");
      _showLaunchpadToast(pick3(`已固定到新建页 2×2 (${res.count}/4)`, `Pinned to 2×2 (${res.count}/4)`, `2×2 に固定 (${res.count}/4)`));
    } else if (res.status === "full") {
      if (typeof triggerHaptic === "function") triggerHaptic("medium");
      _openLaunchpadConfiguratorModal(item);
    }
  };

  // 点击空白区收起移动端键盘进入探索态
  const blankEl = streamEl.querySelector('[data-lp-blank="1"]');
  if (blankEl && isMobile) {
    blankEl.addEventListener("click", (e) => {
      if (e.target.closest("button")) return;
      document.getElementById("chat-input")?.blur();
      _launchpadKeyboardFocused = false;
      window.renderNewChatLaunchpad(streamEl, draft);
    });
  }

  streamEl.querySelectorAll("[data-lp-pool]").forEach((btn) => {
    const poolKey = btn.getAttribute("data-lp-pool");
    const idx = parseInt(btn.getAttribute("data-lp-idx"), 10);
    const card = pools[poolKey]?.[idx];
    const allowPin = poolKey !== "focus";

    let lpTimer = null;
    let lpTriggered = false;
    if (allowPin) {
      btn.addEventListener("pointerdown", (e) => {
        if (e.button && e.button !== 0) return;
        lpTriggered = false;
        clearTimeout(lpTimer);
        lpTimer = setTimeout(() => {
          lpTriggered = true;
          handlePinAction(card);
        }, 450);
      });
      const cancelLp = () => {
        if (lpTimer) {
          clearTimeout(lpTimer);
          lpTimer = null;
        }
      };
      btn.addEventListener("pointerup", cancelLp);
      btn.addEventListener("pointercancel", cancelLp);
      btn.addEventListener("pointermove", cancelLp);
      btn.addEventListener("contextmenu", (e) => {
        e.preventDefault();
        cancelLp();
        handlePinAction(card);
      });
    }

    btn.addEventListener("click", (e) => {
      if (lpTriggered) {
        lpTriggered = false;
        e.preventDefault();
        return;
      }
      triggerCard(card);
    });
  });

  streamEl.querySelector('[data-lp-action="enter-explore"]')?.addEventListener("click", () => {
    document.getElementById("chat-input")?.blur();
    _launchpadKeyboardFocused = false;
    window.renderNewChatLaunchpad(streamEl, draft);
  });
  streamEl.querySelector('[data-lp-action="toggle-mode"]')?.addEventListener("click", () => {
    _launchpadDesktopExploreMode = !_launchpadDesktopExploreMode;
    window.renderNewChatLaunchpad(streamEl, draft);
  });
  streamEl.querySelector('[data-lp-action="open-config"]')?.addEventListener("click", () => {
    _openLaunchpadConfiguratorModal(null);
  });
  streamEl.querySelector('[data-lp-action="shuffle"]')?.addEventListener("click", () => {
    if (typeof triggerHaptic === "function") triggerHaptic("light");
    _launchpadShuffleSeed += 1;
    window.renderNewChatLaunchpad(streamEl, draft);
  });
  streamEl.querySelector('[data-lp-action="switch-ws"]')?.addEventListener("click", () => {
    if (typeof openNewSheet === "function") openNewSheet();
  });
  streamEl.querySelector('[data-lp-action="dirty"]')?.addEventListener("click", () => {
    const reviewPrompt = pick3(
      "请先检查当前未提交的改动，指出潜在风险并给出优化建议",
      "Please review the current uncommitted changes, highlight potential risks, and suggest improvements",
      "未コミットの変更点を確認し、潜在リスクと改善案を教えてください"
    );
    const inputEl = document.getElementById("chat-input");
    if (inputEl) {
      inputEl.value = reviewPrompt;
      inputEl.style.height = "auto";
      inputEl.style.height = Math.min(inputEl.scrollHeight, 120) + "px";
      inputEl.focus();
    }
  });
};

window.refreshNewChatLaunchpad = function () {
  if (typeof activeDraftSession === "undefined" || !activeDraftSession || activeCascadeId) return;
  const streamEl = document.getElementById("messages-stream");
  if (!streamEl) return;
  if (streamEl.querySelector(".launchpad-root") || streamEl.querySelector(".chat-empty-state")) {
    window.renderNewChatLaunchpad(streamEl, activeDraftSession);
  }
};
