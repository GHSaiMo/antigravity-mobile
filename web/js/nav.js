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
      const launchpadEnabled = !(window.NewChatLaunchpadPrefs && !window.NewChatLaunchpadPrefs.isEnabled());
      // 默认开启快捷启动台时不自动聚焦拉起软键盘；仅在关闭启动台或已有预填文本时聚焦。
      if ((!launchpadEnabled || prefillText) && window.matchMedia && window.matchMedia("(pointer: fine)").matches) {
        setTimeout(() => {
          chatInput.focus();
          if (prefillText && typeof chatInput.setSelectionRange === "function") {
            const len = chatInput.value.length;
            chatInput.setSelectionRange(len, len);
          }
        }, 120);
      }
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
// 新建对话轻启动台 (方案 A · 与 iOS / Android 统一规范)
// ==========================================================================

window.NewChatLaunchpadPrefs = {
  isEnabled() {
    return localStorage.getItem("agy_new_chat_launchpad") !== "false";
  }
};

const _launchpadDraftGitCache = {};
let _launchpadKeyboardFocused = false;
let _launchpadListenersBound = false;

function _bindLaunchpadKeyboardListeners() {
  if (_launchpadListenersBound) return;
  _launchpadListenersBound = true;
  const inputEl = document.getElementById("chat-input");
  if (!inputEl) return;
  inputEl.addEventListener("focus", () => {
    if (window.innerWidth <= 820) {
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

function _launchpadGreeting(accountName, compact) {
  const hour = new Date().getHours();
  const isEnLang = typeof isEn === "function" && isEn();
  const isJaLang = typeof isJa === "function" && isJa();
  const period = hour >= 5 && hour < 12
    ? (isEnLang ? "Good morning" : (isJaLang ? "おはようございます" : "上午好"))
    : (hour >= 12 && hour < 18
      ? (isEnLang ? "Good afternoon" : (isJaLang ? "こんにちは" : "下午好"))
      : (isEnLang ? "Good evening" : (isJaLang ? "こんばんは" : "晚上好")));
  const raw = (accountName || "").trim();
  const shortName = raw ? raw.split(/\s+/)[0] : "";
  if (compact) {
    return shortName ? `${period}, ${shortName}` : period;
  }
  if (isEnLang) return shortName ? `${period}, what's next, ${shortName}?` : `${period}, what's next?`;
  if (isJaLang) return shortName ? `${period}、${shortName}さん。次は何をしますか？` : `${period}、次は何をしますか？`;
  return shortName ? `${period}，接下来做什么，${shortName}？` : `${period}，接下来做什么？`;
}

function _buildLaunchpadCards(isPure, dirtyCount) {
  const tr = (s) => (typeof t === "function" ? t(s) : s);
  const isEnLang = typeof isEn === "function" && isEn();
  const isJaLang = typeof isJa === "function" && isJa();
  const cards = [];

  if (!isPure) {
    if (dirtyCount > 0) {
      cards.push({
        icon: "🔍",
        title: isEnLang ? `Review ${dirtyCount} changes` : (isJaLang ? `変更 ${dirtyCount} 件をレビュー` : `审查当前 ${dirtyCount} 个改动`),
        sub: isEnLang ? "Inspect git diff & suggestions" : (isJaLang ? "git diff のリスクと改善確認" : "检查 git diff 风险与优化"),
        prompt: isEnLang
          ? "Please review the current uncommitted changes, highlight potential risks, and suggest improvements"
          : (isJaLang ? "未コミットの変更点を確認し、潜在リスクと改善案を教えてください" : "请先检查当前未提交的改动，指出潜在风险并给出优化建议")
      });
    } else {
      cards.push({
        icon: "🧭",
        title: isEnLang ? "Map Project Structure" : (isJaLang ? "プロジェクト構造を把握" : "梳理项目结构"),
        sub: isEnLang ? "Find core modules & entry points" : (isJaLang ? "主要モジュールと入口を整理" : "快速定位核心模块与入口"),
        prompt: isEnLang
          ? "Please map out the core modules and key entry points of this workspace"
          : (isJaLang ? "このワークスペースの主要モジュールとエントリポイントを整理してください" : "请先梳理这个工作区的核心模块与关键入口文件")
      });
    }
  }

  // 优先承接用户放入快捷菜单的自定义 Workflow 模板
  const defaultIds = new Set(["code_review", "test_and_fix", "research", "custom"]);
  const maxPinned = isPure ? 1 : 2;
  const templates = (typeof workflowState !== "undefined" && Array.isArray(workflowState.templates))
    ? workflowState.templates
    : (typeof loadCachedWorkflowTemplates === "function" ? loadCachedWorkflowTemplates() : []);
  templates
    .filter((tpl) => tpl && tpl.inQuickMenu && !defaultIds.has(tpl.id))
    .slice(0, maxPinned)
    .forEach((tpl) => {
      cards.push({
        icon: "📌",
        title: tpl.title || tr("置顶模板"),
        sub: tpl.subtitle || tr("置顶模板"),
        prompt: tpl.templatePrompt || ""
      });
    });

  const fallback = isPure ? [
    {
      icon: "💬",
      title: isEnLang ? "/grill-me Discuss" : (isJaLang ? "/grill-me 壁打ち" : "/grill-me 探讨"),
      sub: isEnLang ? "Unpack ideas step by step" : (isJaLang ? "アイデアを段階的に深掘り" : "逐项拆解模糊想法"),
      prompt: "/grill-me "
    },
    {
      icon: "🌐",
      title: isEnLang ? "Web Fact-Check" : (isJaLang ? "最新情報を調査" : "联网核查资讯"),
      sub: isEnLang ? "Verify latest updates online" : (isJaLang ? "Web で事実関係を確認" : "交叉验证最新进展"),
      prompt: isEnLang ? "Please verify the latest updates and reliable sources for: " : (isJaLang ? "以下の件の最新状況と信頼できる情報源を調べてください：" : "请帮我联网核查以下消息的最新进展与可靠来源：")
    },
    {
      icon: "✍️",
      title: isEnLang ? "Draft & Polish" : (isJaLang ? "文章の下書き・推敲" : "起草与润色"),
      sub: isEnLang ? "Articles, notes, or outlines" : (isJaLang ? "記事・お知らせ・構成案" : "文章 / 公告 / 提纲"),
      prompt: isEnLang ? "Help me draft a clear and concise write-up for: " : (isJaLang ? "次の内容について分かりやすい説明文を作成してください：" : "帮我起草一份清晰自然的说明：")
    },
    {
      icon: "⚖️",
      title: isEnLang ? "Compare Options" : (isJaLang ? "案を比較検討" : "多维方案对比"),
      sub: isEnLang ? "Weigh pros, cons & tradeoffs" : (isJaLang ? "利点・コスト・リスクを比較" : "权衡利弊与取舍"),
      prompt: isEnLang ? "Please compare these options across pros, cons, cost, and risk: " : (isJaLang ? "以下の案をメリット・コスト・リスクの観点から比較してください：" : "请从利弊、成本与风险角度对比以下方案：")
    }
  ] : [
    {
      icon: "🗺️",
      title: isEnLang ? "/plan Architect" : (isJaLang ? "/plan 計画設計" : "/plan 方案规划"),
      sub: isEnLang ? "Align steps before coding" : (isJaLang ? "手順を固めてから実装" : "先对齐步骤再动手"),
      prompt: "/plan "
    },
    {
      icon: "💬",
      title: isEnLang ? "/grill-me Discuss" : (isJaLang ? "/grill-me 壁打ち" : "/grill-me 探讨"),
      sub: isEnLang ? "Walk through design choices" : (isJaLang ? "要件と設計を一つずつ整理" : "逐项拆解需求决策"),
      prompt: "/grill-me "
    },
    {
      icon: "🐛",
      title: isEnLang ? "Debug an Issue" : (isJaLang ? "バグ原因を調査" : "定位排查 Bug"),
      sub: isEnLang ? "Trace root cause from logs" : (isJaLang ? "ログとコードから原因特定" : "结合上下文与日志定位"),
      prompt: isEnLang ? "Please help investigate and find the root cause of this issue: " : (isJaLang ? "以下の問題の根本原因を調査してください：" : "请帮我排查并定位以下问题的根因：")
    },
    {
      icon: "✨",
      title: isEnLang ? "Refactor & Polish" : (isJaLang ? "改善とリファクタ" : "优化与重构"),
      sub: isEnLang ? "Improve clarity & stability" : (isJaLang ? "可読性と安定性を向上" : "提升可读性与稳定性"),
      prompt: isEnLang ? "Please identify improvements in the current module and propose a lightweight refactor: " : (isJaLang ? "現在のモジュールの改善点を分析し、軽量なリファクタ案を提案してください：" : "请分析当前模块可优化的设计点并给出轻量重构方案：")
    }
  ];

  for (const item of fallback) {
    if (cards.length >= 4) break;
    cards.push(item);
  }
  return cards.slice(0, 4);
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

  // 静默预热 Token 历史与草稿 Git 状态（零闪烁）
  if (typeof refreshTokenHistoryIfStale === "function" && (typeof currentTokenHistory === "undefined" || !currentTokenHistory)) {
    try { refreshTokenHistoryIfStale(); } catch {}
  }
  if (!isPure && draft.uri) {
    _fetchDraftGitStatusIfNeeded(draft.uri);
  }

  const quotaData = typeof currentCockpitQuotas !== "undefined" ? currentCockpitQuotas : null;
  const currentAcc = quotaData && (quotaData.current_account || (quotaData.accounts && quotaData.accounts[0]));
  const resolvedQuota = (currentAcc && typeof resolveAccountQuotaDisplay === "function")
    ? resolveAccountQuotaDisplay(currentAcc, typeof selectedQuotaBucket !== "undefined" ? selectedQuotaBucket : "gemini_5h")
    : null;
  const quotaPct = resolvedQuota && resolvedQuota.bucket ? Math.max(0, Math.min(100, Math.round(resolvedQuota.bucket.remaining_percent))) : null;
  const quotaClass = quotaPct == null ? "" : (quotaPct >= 50 ? "good" : (quotaPct >= 20 ? "warn" : "danger"));
  const quotaShortLabel = resolvedQuota
    ? (resolvedQuota.key === "gemini_5h" ? "Gemini 5h"
      : resolvedQuota.key === "gemini_weekly" ? "Gemini"
      : resolvedQuota.key === "claude_5h" ? "Claude 5h"
      : resolvedQuota.key === "claude_weekly" ? "Claude" : "")
    : "";

  const tokenData = typeof currentTokenHistory !== "undefined" ? currentTokenHistory : null;
  const todayTokens = tokenData?.summary?.today_tokens > 0 && typeof formatTokensCompact === "function"
    ? formatTokensCompact(tokenData.summary.today_tokens) : "";
  const cacheHit = tokenData && typeof computeTokenCacheHitRate === "function"
    ? computeTokenCacheHitRate(tokenData) : null;
  const bookEq = tokenData && typeof resolveTokenBookEquivalent === "function"
    ? resolveTokenBookEquivalent(tokenData) : null;

  const gitInfo = (!isPure && draft.uri) ? (_launchpadDraftGitCache[draft.uri] || {}) : {};
  const gitBranch = gitInfo.branch || "";
  const dirtyCount = gitInfo.dirtyCount || 0;
  const compact = _launchpadKeyboardFocused && window.innerWidth <= 820;

  const isEnLang = typeof isEn === "function" && isEn();
  const isJaLang = typeof isJa === "function" && isJa();
  const todayLabel = isEnLang ? "Today" : (isJaLang ? "今日" : "今日");
  const cacheLabel = isEnLang ? "Cache" : (isJaLang ? "キャッシュ" : "缓存");
  const quickLabel = isEnLang ? "Quick Start" : (isJaLang ? "クイック開始" : "快捷发起");
  const switchLabel = isEnLang ? "Switch" : (isJaLang ? "切替" : "切换");
  const pickWsLabel = isEnLang ? "Workspace" : (isJaLang ? "選択" : "选工作区");
  const cleanLabel = isEnLang ? "Clean" : (isJaLang ? "クリーン" : "干净");
  const pureDesc = isEnLang ? "Pure chat · No workspace linked" : (isJaLang ? "チャット専用 · ワークスペース未連携" : "纯对话模式 · 不关联代码工作区");
  const dirtyLabel = isEnLang ? `${dirtyCount} dirty` : (isJaLang ? `${dirtyCount} 変更` : `${dirtyCount} 改动`);

  const hasStatus = quotaPct !== null || !!todayTokens || cacheHit !== null;
  const cards = _buildLaunchpadCards(isPure, dirtyCount);
  const greeting = _launchpadGreeting(currentAcc?.name, compact);

  if (compact) {
    streamEl.innerHTML = `
      <div class="launchpad-root launchpad-compact">
        <div class="launchpad-compact-head">
          <div class="launchpad-header">
            <span class="launchpad-brand">✦</span>
            <span class="launchpad-greeting">${escapeHtml(greeting)}</span>
          </div>
          ${hasStatus ? `
            <button type="button" class="launchpad-mini-pill" data-lp-action="quota">
              ${quotaPct !== null ? `<span class="lp-q-val ${quotaClass}">⚡${quotaPct}%</span>` : ""}
              ${todayTokens ? `<span class="lp-muted">${quotaPct !== null ? "· " : ""}${escapeHtml(todayTokens)}</span>` : ""}
            </button>
          ` : ""}
        </div>
        <div class="launchpad-compact-strip">
          ${cards.map((c, idx) => `
            <button type="button" class="launchpad-compact-chip" data-lp-card="${idx}">
              <span>${c.icon}</span>
              <span>${escapeHtml(c.title)}</span>
            </button>
          `).join("")}
        </div>
      </div>
    `;
  } else {
    const rightParts = [];
    if (todayTokens) rightParts.push(`${todayLabel} ${todayTokens}`);
    if (cacheHit !== null) rightParts.push(`${cacheLabel} ${cacheHit}%`);

    streamEl.innerHTML = `
      <div class="launchpad-root">
        <div class="launchpad-header">
          <span class="launchpad-brand">✦</span>
          <span class="launchpad-greeting">${escapeHtml(greeting)}</span>
        </div>

        ${hasStatus ? `
          <button type="button" class="launchpad-status-strip" data-lp-action="quota">
            <div class="lp-status-left">
              ${quotaPct !== null ? `
                <span class="lp-q-bolt ${quotaClass}">⚡</span>
                ${quotaShortLabel ? `<span class="lp-q-name">${escapeHtml(quotaShortLabel)}</span>` : ""}
                <span class="lp-q-val ${quotaClass}">${quotaPct}%</span>
                <span class="lp-q-bar"><span class="lp-q-fill ${quotaClass}" style="width:${Math.max(4, quotaPct)}%"></span></span>
              ` : ""}
              ${rightParts.length ? `<span class="lp-status-meta">${escapeHtml(rightParts.join(" · "))}</span>` : ""}
            </div>
            <span class="lp-chevron">›</span>
          </button>
        ` : ""}

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

        <div class="launchpad-section">
          <div class="launchpad-section-title">${escapeHtml(quickLabel)}</div>
          <div class="launchpad-grid">
            ${cards.map((c, idx) => `
              <button type="button" class="launchpad-card" data-lp-card="${idx}">
                <div class="launchpad-card-title"><span>${c.icon}</span><span>${escapeHtml(c.title)}</span></div>
                <div class="launchpad-card-sub">${escapeHtml(c.sub)}</div>
              </button>
            `).join("")}
          </div>
        </div>

        ${bookEq ? `
          <button type="button" class="launchpad-book-btn" data-lp-action="history">
            <span>✨</span>
            <span>${escapeHtml(bookEq.sentence)}</span>
            <span class="lp-chevron">›</span>
          </button>
        ` : ""}
      </div>
    `;
  }

  streamEl.querySelectorAll("[data-lp-card]").forEach((btn) => {
    btn.addEventListener("click", () => {
      const idx = parseInt(btn.getAttribute("data-lp-card"), 10);
      const card = cards[idx];
      if (!card) return;
      const inputEl = document.getElementById("chat-input");
      if (inputEl) {
        inputEl.value = card.prompt;
        inputEl.style.height = "auto";
        inputEl.style.height = Math.min(inputEl.scrollHeight, 120) + "px";
        inputEl.focus();
        if (typeof inputEl.setSelectionRange === "function") {
          const openIdx = card.prompt.indexOf("[");
          const closeIdx = openIdx >= 0 ? card.prompt.indexOf("]", openIdx) : -1;
          if (openIdx >= 0 && closeIdx > openIdx) {
            inputEl.setSelectionRange(openIdx, closeIdx + 1);
          } else {
            const len = card.prompt.length;
            inputEl.setSelectionRange(len, len);
          }
        }
      }
    });
  });

  streamEl.querySelector('[data-lp-action="quota"]')?.addEventListener("click", () => {
    if (typeof openQuotaSheet === "function") openQuotaSheet("realtime");
  });
  streamEl.querySelector('[data-lp-action="history"]')?.addEventListener("click", () => {
    if (typeof openQuotaSheet === "function") openQuotaSheet("history");
  });
  streamEl.querySelector('[data-lp-action="switch-ws"]')?.addEventListener("click", () => {
    if (typeof openNewSheet === "function") openNewSheet();
  });
  streamEl.querySelector('[data-lp-action="dirty"]')?.addEventListener("click", () => {
    const reviewPrompt = isEnLang
      ? "Please review the current uncommitted changes, highlight potential risks, and suggest improvements"
      : (isJaLang ? "未コミットの変更点を確認し、潜在リスクと改善案を教えてください" : "请先检查当前未提交的改动，指出潜在风险并给出优化建议");
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
