// --- Navigation & Routing ---

function formatConversationTitle(annotations, summary, fallback = "未命名会话") {
  const raw = annotations?.title || summary;
  if (!raw || raw === "未命名会话") return fallback;
  const cleaned = raw.replace(/<[^>]+>/g, "").trim();
  const firstLine = cleaned.split("\n").map((l) => l.trim()).find((l) => l.length > 0) || "";
  if (!firstLine) return fallback;
  if (firstLine.length > 36) {
    return firstLine.slice(0, 36) + "...";
  }
  return firstLine;
}

function navigateTo(hash) {
  window.location.hash = hash;
  renderRoute();
}

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

  // Persist current input text to memory draft before leaving the session
  const chatInput = document.getElementById("chat-input");
  if (activeCascadeId && chatInput) {
    DraftManager.set(activeCascadeId, chatInput.value);
  }

  if (hash.startsWith("#c=")) {
    const newCascadeId = hash.slice(3);
    const changed = activeCascadeId !== newCascadeId;
    activeCascadeId = newCascadeId;
    if (changed) SubagentManager.reset(); // 切换会话时清掉上一个会话的子代理 / 只读状态
    markConversationAsRead(newCascadeId);

    // View toggling with iOS NavigationStack slide
    convView.classList.add("pushed-left");
    chatView.classList.add("active");

    // Nav Bar configuration for Chat View
    if (settingsBtn) settingsBtn.classList.add("hidden");
    if (newBtn) newBtn.classList.add("hidden");
    if (backBtn) backBtn.classList.remove("hidden");
    if (brandHeader) brandHeader.classList.add("hidden");
    if (inlineTitle) inlineTitle.classList.remove("hidden");

    // Title resolution
    const summary = currentTrajectories[activeCascadeId];
    const title = formatConversationTitle(summary?.annotations, summary?.summary, "会话详情");
    const wsUri = summary?.workspaceUris?.[0] || summary?.workspaces?.[0]?.workspaceFolderAbsoluteUri || "";
    const wsName = wsUri.split("/").filter(Boolean).pop() || "";

    if (titleText) titleText.textContent = title;
    if (wsText) wsText.textContent = wsName ? `📁 ${wsName}` : "";

    if (changed) {
      hasInitiallyAligned = false;
      prevWasRunning = false;
      updatePendingInteraction(null, false);

      // Restore session draft into chat input
      if (chatInput) {
        chatInput.value = DraftManager.get(newCascadeId);
        chatInput.style.height = "auto";
        chatInput.style.height = Math.min(chatInput.scrollHeight, 120) + "px";
        const isRunning = currentTrajectories[activeCascadeId]?.status === "CASCADE_RUN_STATUS_RUNNING";
        updateChatControls(isRunning, null, false);
      }

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

      // Fetch initial conversation trajectory via HTTP RPC only if not cached in memory
      if (!sessionStepsCache[activeCascadeId] || !sessionStepsCache[activeCascadeId].steps?.length) {
        loadChat(activeCascadeId, true);
      }
    }

    // Connect real-time WebSocket stream
    connectStreamWs(activeCascadeId);
  } else if (hash === "#draft") {
    if (!activeDraftSession) {
      activeDraftSession = { isPure: true, name: "新对话", path: "", uri: "", rawId: "outside-of-project" };
    }
    activeCascadeId = null;
    closeActiveWs(true);
    updatePendingInteraction(null, false);

    convView.classList.add("pushed-left");
    chatView.classList.add("active");

    if (settingsBtn) settingsBtn.classList.add("hidden");
    if (newBtn) newBtn.classList.add("hidden");
    if (backBtn) backBtn.classList.remove("hidden");
    if (brandHeader) brandHeader.classList.add("hidden");
    if (inlineTitle) inlineTitle.classList.remove("hidden");

    if (titleText) titleText.textContent = activeDraftSession.isPure ? "新对话" : activeDraftSession.name;
    if (wsText) wsText.textContent = activeDraftSession.isPure ? "Chat" : `📁 ${activeDraftSession.name}`;

    const streamEl = document.getElementById("messages-stream");
    if (streamEl) {
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

    if (chatInput) {
      chatInput.value = "";
      chatInput.style.height = "auto";
      setTimeout(() => chatInput.focus(), 250);
    }
    updateChatControls(false, activeDraftSession.isPure ? "" : activeDraftSession.uri, false);
  } else {
    activeCascadeId = null;
    activeDraftSession = null;
    closeActiveWs(true);
    updatePendingInteraction(null, false);
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
    if (backBtn) backBtn.classList.add("hidden");
    if (brandHeader) brandHeader.classList.remove("hidden");
    if (inlineTitle) inlineTitle.classList.add("hidden");

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
  listEventsAttempts = 0;
  if (listEventsRefreshTimer) {
    clearTimeout(listEventsRefreshTimer);
    listEventsRefreshTimer = null;
  }
  closeListEventsSocket();
}

function startListEvents() {
  listEventsWanted = true;
  connectListEvents();
}

async function connectListEvents() {
  if (!listEventsWanted || document.visibilityState !== "visible") return;
  if (listEventsWs && (listEventsWs.readyState === WebSocket.OPEN || listEventsWs.readyState === WebSocket.CONNECTING)) return;
  closeListEventsSocket();

  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  let wsUrl = `${proto}//${location.host}/gateway/events`;
  try {
    // Same one-time ticket exchange as the cascade stream: no long-lived token in the URL.
    const resp = await originalFetch("/api/v1/auth/ws-ticket", { method: "POST" });
    if (resp.ok) {
      const data = await resp.json();
      if (data && data.ticket) wsUrl += `?ticket=${encodeURIComponent(data.ticket)}`;
    }
  } catch (_) {}
  // The route may have changed while the ticket was in flight.
  if (!listEventsWanted || document.visibilityState !== "visible") return;
  closeListEventsSocket();

  try {
    const ws = new WebSocket(wsUrl);
    listEventsWs = ws;
    ws.onopen = () => {
      listEventsAttempts = 0;
    };
    ws.onmessage = (event) => {
      let frame;
      try { frame = JSON.parse(event.data); } catch (_) { return; }
      if (frame.type === "changed") {
        scheduleListRefresh();
      } else if (frame.type === "hello") {
        // After a reconnect we may have missed changes; the first connect already has fresh data.
        if (listEventsHadConnection) scheduleListRefresh();
        listEventsHadConnection = true;
      }
    };
    ws.onclose = () => {
      if (listEventsWs !== ws) return;
      listEventsWs = null;
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
      connectListEvents();
      if (isConversationListActive()) scheduleListRefresh();
    }
  } else {
    closeListEventsSocket();
  }
});

async function loadConversations(quiet = false) {
  const listEl = document.getElementById("conversations-list");
  refreshGatewayCompat();
  try {
    const data = await rpc("GetAllCascadeTrajectories");
    const summaries = data.trajectorySummaries || {};
    currentTrajectories = summaries;

    renderConversationList(summaries);
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

// 子代理会话不在列表里展示（与原生端一致）：有父会话 / 战斗模式分叉 / 子代理规格 / 嵌套深度 > 0。
function isSubagentSummary(info) {
  const meta = info && info.trajectoryMetadata;
  if (!meta) return false;
  if (typeof meta.parentConversationId === "string" && meta.parentConversationId.trim()) return true;
  if (meta.isBattleModeFork === true) return true;
  if (meta.subagentSpec) return true;
  if (meta.agentScript) return true;
  if (typeof meta.nestingDepth === "number" && meta.nestingDepth > 0) return true;
  return false;
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

function renderConversationList(summaries) {
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

  const items = Object.entries(summaries)
    .filter(([, info]) => !isSubagentSummary(info))
    .map(([id, info]) => ({ id, ...info }))
    .sort((a, b) => new Date(b.lastModifiedTime || 0) - new Date(a.lastModifiedTime || 0))
    .filter((item) => {
      if (!query) return true;
      const title = formatConversationTitle(item.annotations, item.summary, "").toLowerCase();
      const ws = (item.workspaceUris?.[0] || "").toLowerCase();
      return title.includes(query) || ws.includes(query) || item.id.includes(query);
    });

  if (items.length === 0) {
    // 有内容命中（或正在搜索）时不显示「暂无匹配」，由下面的内容命中区承接
    const contentBusy = !!query && contentSearch.query === (searchInput?.value || "").trim() && (contentSearch.loading || contentSearch.results.length > 0);
    listEl.innerHTML = `
      <div class="loading-state no-match-state${contentBusy ? " hidden" : ""}">
        <p>暂无匹配会话</p>
      </div>
    `;
    return;
  }

  listEl.innerHTML = items
    .map((item) => {
      const hasError = !!item.hasError || item.status === "CASCADE_RUN_STATUS_ERROR";
      const hasAction = !hasError && !!item.needsInput;
      const isRunning = !hasError && item.status === "CASCADE_RUN_STATUS_RUNNING" && !hasAction;
      const hasDraft = !hasError && !hasAction && !isRunning && DraftManager.has(item.id);
      const isUnread = !hasError && !isRunning && !hasAction && !hasDraft && isConversationUnread(item);
      const unreadDotHtml = isUnread
        ? `<div class="status-unread-dot" title="未读新消息" data-testid="status-unread-dot"><div class="dot-halo"></div><div class="dot-core"></div></div>`
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
      const title = formatConversationTitle(item.annotations, item.summary, "未命名会话");
      const wsUri = item.workspaceUris?.[0] || item.workspaces?.[0]?.workspaceFolderAbsoluteUri || "";
      const wsName = wsUri.split("/").filter(Boolean).pop() || "Chat";
      const timeStr = formatRelativeTime(item.lastModifiedTime);

      return `
        <div class="conv-card-wrapper" data-id="${item.id}">
          <div class="conv-card-actions">
            <button class="conv-card-delete-btn" type="button" aria-label="删除会话" data-id="${item.id}">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="3 6 5 6 21 6"></polyline>
                <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path>
              </svg>
            </button>
          </div>
          <div class="conv-card" data-id="${item.id}" data-title="${escapeHtml(title)}">
            <div class="conv-card-top">
              <div class="conv-title">${escapeHtml(title)}</div>
              ${badgeHtml}
            </div>
            <div class="conv-card-bottom">
              <div class="conv-meta">
                ${wsName === "Chat"
                  ? `<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                      <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"></path>
                    </svg>`
                  : `<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                      <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"></path>
                    </svg>`}
                <span class="ws-name">${escapeHtml(wsName)}</span>
              </div>
              <span class="conv-steps-time">${item.stepCount || 0} 步骤 • ${timeStr}</span>
            </div>
          </div>
        </div>
      `;
    })
    .join("");

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
  const listEl = document.getElementById("conversation-list");
  if (!listEl || convDelegationInitialized) return;
  convDelegationInitialized = true;

  listEl.addEventListener("touchstart", (e) => {
    if (e.touches.length > 1) return;
    if (e.target.closest(".conv-card-delete-btn")) return;
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
        openRenameAlert(id, title);
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

      const isAlreadySwiped = card.classList.contains("swiped");
      const baseOffset = isAlreadySwiped ? -80 : 0;
      let newX = baseOffset + dx;

      if (newX > 0) {
        newX = newX * 0.2;
      } else if (newX < -80) {
        newX = -80 + (newX + 80) * 0.25;
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
      const isAlreadySwiped = card.classList.contains("swiped");
      if (isAlreadySwiped) {
        if (dx > 25) {
          closeCard(card);
        } else {
          card.style.transform = "translateX(-80px)";
          card.classList.add("swiped");
          wrapper.classList.add("swiped");
        }
      } else {
        if (dx < -38) {
          card.style.transform = "translateX(-80px)";
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

function closeDeleteActionSheet() {
  pendingDeleteCascadeId = null;
  const sheet = document.getElementById("actionsheet-delete");
  if (sheet) sheet.classList.add("hidden");
}

async function confirmDeleteConversation() {
  const id = pendingDeleteCascadeId;
  closeDeleteActionSheet();
  if (!id) return;

  const wrapper = document.querySelector(`.conv-card-wrapper[data-id="${id}"]`);
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
  const titleEl = document.querySelector(`.conv-card[data-id="${id}"] .conv-title`);
  if (titleEl) titleEl.textContent = newTitle;
  const cardEl = document.querySelector(`.conv-card[data-id="${id}"]`);
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
    if (!activeCascadeId || e.touches.length > 1) return;
    const touchX = e.touches[0].clientX;
    // Edge trigger zone: within left 32px of the screen
    if (touchX <= 32) {
      startX = touchX;
      startY = e.touches[0].clientY;
      currentX = startX;
      isSwiping = true;
      isHorizontal = null;
      startTime = Date.now();
      chatView.classList.add("is-swiping");
      convView.classList.add("is-swiping");
    }
  }, { passive: true });

  window.addEventListener("touchmove", (e) => {
    if (!isSwiping) return;
    currentX = e.touches[0].clientX;
    const currentY = e.touches[0].clientY;
    const dx = currentX - startX;
    const dy = currentY - startY;

    if (isHorizontal === null && (Math.abs(dx) > 5 || Math.abs(dy) > 5)) {
      isHorizontal = Math.abs(dx) > Math.abs(dy);
    }

    if (isHorizontal) {
      const clampedX = Math.max(0, dx);
      chatView.style.transform = `translateX(${clampedX}px)`;

      const progress = Math.min(1, clampedX / window.innerWidth);
      const convOffset = -28 + progress * 28;

      convView.style.transform = `translateX(${convOffset}%)`;
    }
  }, { passive: true });

  window.addEventListener("touchend", () => {
    if (!isSwiping) return;
    isSwiping = false;
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
  });
}

// --- iOS Virtual Viewport & Keyboard Handling ---

function initVisualViewportHandling() {
  if (!window.visualViewport) return;

  const handleViewportChange = () => {
    if (!activeCascadeId) return;
    const vp = window.visualViewport;
    const offset = Math.max(0, window.innerHeight - vp.height - vp.offsetTop);
    const appEl = document.getElementById("app");
    if (!appEl) return;

    if (offset > 60) {
      appEl.style.height = `${vp.height}px`;
      const streamEl = document.getElementById("messages-stream");
      if (streamEl && userIsNearBottom) {
        streamEl.scrollTop = streamEl.scrollHeight;
      }
    } else {
      appEl.style.height = "";
    }
  };

  window.visualViewport.addEventListener("resize", handleViewportChange);
  window.visualViewport.addEventListener("scroll", handleViewportChange);
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
    `<div class="content-hits-header"><span>对话内容匹配</span>${contentSearch.loading ? '<div class="ios-spinner content-hits-spinner"></div>' : ""}</div>` +
    hits
      .map((h) => {
        const title = h.title && h.title.trim() ? h.title : "未命名会话";
        const ws = h.workspaceName ? `<div class="content-hit-ws">${escapeHtml(h.workspaceName)}</div>` : "";
        const snippet = h.snippet ? `<div class="content-hit-snippet">${highlightSnippetHtml(h.snippet, h.snippetMatchRanges)}</div>` : "";
        return `<button type="button" class="content-hit-card" data-id="${escapeHtml(h.cascadeId)}">
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
      contentSearch.results = visible.size ? results.filter((h) => visible.has(h.cascadeId)) : results;
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
    if (id) navigateTo(`#c=${id}`);
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
