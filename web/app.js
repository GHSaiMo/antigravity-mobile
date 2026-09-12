// Antigravity Mobile Gateway - Web & PWA Client

// --- Global Auth Token Interceptor & 401 Handler ---
const originalFetch = window.fetch;
window.fetch = async function (url, options = {}) {
  const token = localStorage.getItem("agy_device_token");
  if (token) {
    options = options || {};
    options.headers = options.headers || {};
    if (options.headers instanceof Headers) {
      if (!options.headers.has("Authorization")) {
        options.headers.set("Authorization", `Bearer ${token}`);
      }
    } else {
      if (!options.headers["Authorization"]) {
        options.headers["Authorization"] = `Bearer ${token}`;
      }
    }
  }

  const response = await originalFetch(url, options);

  // Auto trigger pairing sheet if 401 Unauthorized encountered on protected API routes
  if (response.status === 401 && typeof url === "string" && !url.includes("/api/v1/auth/pair")) {
    console.warn("[Auth] 401 Unauthorized received for:", url);
    localStorage.removeItem("agy_device_token");
    if (typeof updateAuthUI === "function") updateAuthUI();
    if (typeof openPairingSheet === "function") openPairingSheet("设备凭据已失效或被 Mac 网关吊销，请重新配对");
  }

  return response;
};

let activeCascadeId = null;
let pollTimer = null;
let currentTrajectories = {};
let availableModels = [];
const sessionStepsCache = {};

// Active model & Image attachments state
let activeModel = localStorage.getItem("agy_active_model") || "gemini-3.8-flash-high";
let pendingImages = []; // [{ id, name, mimeType, base64Data, dataUrl }]

// iOS Haptic Simulation & App Badge helpers
function triggerHaptic(type = "light") {
  if (navigator.vibrate) {
    try {
      if (type === "light") navigator.vibrate(10);
      else if (type === "selection") navigator.vibrate(8);
      else if (type === "medium") navigator.vibrate(22);
      else if (type === "heavy") navigator.vibrate(40);
      else if (type === "success") navigator.vibrate([12, 45, 18]);
    } catch (_) {}
  }
}

function updateAppBadge(count) {
  if ("setAppBadge" in navigator) {
    if (count > 0) navigator.setAppBadge(count).catch(() => {});
    else navigator.clearAppBadge().catch(() => {});
  }
}

function clearAppBadge() {
  if ("clearAppBadge" in navigator) {
    navigator.clearAppBadge().catch(() => {});
  }
}

function updateAppBadgeFromList(items) {
  let unreadCount = 0;
  for (const item of items) {
    if (item.needsInput || isConversationUnread(item)) {
      unreadCount++;
    }
  }
  updateAppBadge(unreadCount);
}

function updateModelSwitchUI() {
  const btn = document.getElementById("btn-model-switch");
  const text = document.getElementById("model-switch-text");
  if (!btn || !text) return;

  if (activeModel === "claude-opus-4-6-thinking") {
    btn.className = "chip-pill chip-model-switch chip-claude";
    text.textContent = "Claude";
    btn.title = "当前模型: Claude (Opus 4.6 Thinking) - 点击切换为 Gemini";
  } else {
    activeModel = "gemini-3.8-flash-high";
    btn.className = "chip-pill chip-model-switch chip-gemini";
    text.textContent = "Gemini";
    btn.title = "当前模型: Gemini (3.8 Flash High) - 点击切换为 Claude";
  }
}

async function toggleModel() {
  if (activeModel === "gemini-3.8-flash-high") {
    activeModel = "claude-opus-4-6-thinking";
  } else {
    activeModel = "gemini-3.8-flash-high";
  }
  localStorage.setItem("agy_active_model", activeModel);
  updateModelSwitchUI();

  // Keep new-model dropdown in sync if opened
  const newModelSelect = document.getElementById("new-model");
  if (newModelSelect) {
    newModelSelect.value = activeModel;
  }

  // Update Language Server default model via JetboxWriteState
  const modelEnum = (activeModel === "claude-opus-4-6-thinking") ? "MODEL_PLACEHOLDER_M26" : "MODEL_PLACEHOLDER_M318";
  try {
    await rpc("JetboxWriteState", {
      appState: {
        lastSelectedAgentModel: modelEnum
      }
    });
  } catch (err) {
    console.warn("[Model] Failed to sync model to Language Server:", err);
  }
}

function syncActiveModel(rawModel) {
  if (!rawModel) return;
  const lower = rawModel.toLowerCase();
  const target = (lower.includes("claude") || lower.includes("m26"))
    ? "claude-opus-4-6-thinking"
    : "gemini-3.8-flash-high";
  if (activeModel !== target) {
    activeModel = target;
    localStorage.setItem("agy_active_model", activeModel);
    updateModelSwitchUI();
    const newModelSelect = document.getElementById("new-model");
    if (newModelSelect) {
      newModelSelect.value = activeModel;
    }
  }
}


function renderImagePreviews() {
  const bar = document.getElementById("image-previews-bar");
  if (!bar) return;

  if (pendingImages.length === 0) {
    bar.innerHTML = "";
    bar.classList.add("hidden");
    updateChatControls(currentTrajectories[activeCascadeId]?.status === "CASCADE_RUN_STATUS_RUNNING");
    return;
  }

  bar.classList.remove("hidden");
  bar.innerHTML = pendingImages.map(img => `
    <div class="image-preview-item" data-id="${img.id}">
      <img src="${img.dataUrl}" alt="${escapeHtml(img.name || '图片')}" />
      <button class="image-preview-remove" type="button" aria-label="删除图片" onclick="removePendingImage('${img.id}')">✕</button>
    </div>
  `).join("");
  updateChatControls(currentTrajectories[activeCascadeId]?.status === "CASCADE_RUN_STATUS_RUNNING");
}

function removePendingImage(id) {
  pendingImages = pendingImages.filter(img => img.id !== id);
  renderImagePreviews();
}

function handleFilesSelected(files) {
  if (!files || !files.length) return;
  for (const file of Array.from(files)) {
    if (!file.type.startsWith("image/")) continue;
    const reader = new FileReader();
    const id = `img-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
    reader.onload = (e) => {
      const dataUrl = e.target.result;
      const base64Data = dataUrl.split(",")[1];
      pendingImages.push({
        id,
        name: file.name,
        mimeType: file.type || "image/jpeg",
        base64Data,
        dataUrl
      });
      renderImagePreviews();
    };
    reader.readAsDataURL(file);
  }
}

// --- ConnectRPC & Gateway API ---

async function rpc(method, body = {}, extraHeaders = {}) {
  const headers = {
    "Content-Type": "application/json",
    "Connect-Protocol-Version": "1",
    ...extraHeaders
  };
  if (activeModel) {
    headers["X-Antigravity-Model"] = activeModel;
  }
  const resp = await fetch(`/api/exa.language_server_pb.LanguageServerService/${method}`, {
    method: "POST",
    headers: headers,
    body: JSON.stringify(body)
  });

  if (!resp.ok) {
    let errMsg = resp.statusText;
    try {
      const err = await resp.json();
      errMsg = err.message || errMsg;
    } catch (_) {}
    throw new Error(errMsg);
  }
  return resp.json();
}

async function checkGatewayStatus() {
  const statusPill = document.getElementById("settings-status-pill");
  const portEl = document.getElementById("settings-upstream-port");
  const pidEl = document.getElementById("settings-upstream-pid");
  const tokenEl = document.getElementById("settings-csrf-token");
  const chatDot = document.getElementById("chat-status-dot");

  try {
    const resp = await fetch("/gateway/status");
    const data = await resp.json();

    if (data.status === "connected" && data.upstream) {
      if (statusPill) {
        statusPill.className = "status-badge connected";
        statusPill.textContent = "已连接";
      }
      if (portEl) portEl.textContent = data.upstream.port;
      if (pidEl) pidEl.textContent = data.upstream.pid;
      if (tokenEl) tokenEl.textContent = data.upstream.csrf_token || "-";
      if (chatDot) {
        chatDot.classList.add("active");
        chatDot.title = `已连接 :${data.upstream.port}`;
      }
    } else {
      if (statusPill) {
        statusPill.className = "status-badge disconnected";
        statusPill.textContent = "未连接";
      }
      if (chatDot) {
        chatDot.classList.remove("active");
        chatDot.title = "上游未连接";
      }
    }
  } catch (e) {
    if (statusPill) {
      statusPill.className = "status-badge disconnected";
      statusPill.textContent = "网关离线";
    }
    if (chatDot) {
      chatDot.classList.remove("active");
      chatDot.title = "网关离线";
    }
  }
}

async function rescanGateway() {
  const btn = document.getElementById("btn-rescan-gateway");
  if (btn) btn.textContent = "正在重新探测...";

  try {
    const resp = await fetch("/gateway/rescan");
    await resp.json();
    await checkGatewayStatus();
    loadConversations();
  } catch (e) {
    await checkGatewayStatus();
  } finally {
    if (btn) btn.textContent = "重新嗅探 Antigravity 实例";
  }
}

// --- Navigation & Routing ---

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
  const inlineTitle = document.getElementById("nav-inline-title");
  const titleText = document.getElementById("chat-title-text");
  const wsText = document.getElementById("chat-workspace-text");
  const chatDot = document.getElementById("chat-status-dot");

  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }

  if (hash.startsWith("#c=")) {
    const newCascadeId = hash.slice(3);
    const changed = activeCascadeId !== newCascadeId;
    activeCascadeId = newCascadeId;
    markConversationAsRead(newCascadeId);

    // View toggling with iOS NavigationStack slide
    convView.classList.add("pushed-left");
    chatView.classList.add("active");

    // Nav Bar configuration for Chat View
    if (settingsBtn) settingsBtn.classList.add("hidden");
    if (newBtn) newBtn.classList.add("hidden");
    if (backBtn) backBtn.classList.remove("hidden");
    if (inlineTitle) inlineTitle.classList.remove("hidden");
    if (chatDot) chatDot.classList.remove("hidden");

    // Title resolution
    const summary = currentTrajectories[activeCascadeId];
    const title = summary?.annotations?.title || summary?.summary || "会话详情";
    const wsUri = summary?.workspaceUris?.[0] || summary?.workspaces?.[0]?.workspaceFolderAbsoluteUri || "";
    const wsName = wsUri.split("/").filter(Boolean).pop() || "";

    if (titleText) titleText.textContent = title;
    if (wsText) wsText.textContent = wsName ? `📁 ${wsName}` : "";

    if (changed) {
      hasInitiallyAligned = false;
      prevWasRunning = false;
      updatePendingInteraction(null, false);
      const streamEl = document.getElementById("messages-stream");
      const cached = sessionStepsCache[activeCascadeId];
      if (cached && cached.steps && cached.steps.length > 0) {
        renderMessages(cached.steps, cached.isRunning);
      } else if (streamEl) {
        streamEl.innerHTML = `
          <div class="loading-state">
            <div class="ios-spinner"></div>
            <p>正在同步会话历史与步骤...</p>
          </div>
        `;
      }
    }

    // Connect real-time WebSocket stream
    connectStreamWs(activeCascadeId);
  } else {
    activeCascadeId = null;
    closeActiveWs();
    updatePendingInteraction(null, false);

    // View toggling with iOS NavigationStack pop
    chatView.classList.remove("active");
    convView.classList.remove("pushed-left");
    clearAppBadge();

    // Nav Bar configuration for List View
    if (settingsBtn) settingsBtn.classList.remove("hidden");
    if (newBtn) newBtn.classList.remove("hidden");
    if (backBtn) backBtn.classList.add("hidden");
    if (inlineTitle) inlineTitle.classList.add("hidden");
    if (chatDot) chatDot.classList.add("hidden");

    loadConversations();
  }
}

// --- Conversations List ---

async function loadConversations() {
  const listEl = document.getElementById("conversations-list");
  try {
    const data = await rpc("GetAllCascadeTrajectories");
    const summaries = data.trajectorySummaries || {};
    currentTrajectories = summaries;

    renderConversationList(summaries);
  } catch (err) {
    listEl.innerHTML = `
      <div class="loading-state">
        <p style="color: var(--ios-red);">加载失败: ${escapeHtml(err.message)}</p>
        <button class="btn-ios-secondary" onclick="loadConversations()" style="margin-top:10px;">点击重试</button>
      </div>
    `;
  }
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
    .map(([id, info]) => ({ id, ...info }))
    .sort((a, b) => new Date(b.lastModifiedTime || 0) - new Date(a.lastModifiedTime || 0))
    .filter((item) => {
      if (!query) return true;
      const title = (item.annotations?.title || item.summary || "").toLowerCase();
      const ws = (item.workspaceUris?.[0] || "").toLowerCase();
      return title.includes(query) || ws.includes(query) || item.id.includes(query);
    });

  if (items.length === 0) {
    listEl.innerHTML = `
      <div class="loading-state">
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
      const isUnread = !hasError && !isRunning && !hasAction && isConversationUnread(item);
      const unreadDotHtml = isUnread
        ? `<div class="status-unread-dot" title="未读新消息" data-testid="status-unread-dot"><div class="dot-halo"></div><div class="dot-core"></div></div>`
        : "";
      const badgeHtml = hasError
        ? `<span class="badge badge-error">error</span>`
        : (hasAction
          ? `<span class="badge badge-action">ACTION</span>`
          : (isRunning ? `<span class="badge badge-running">RUNNING</span>` : unreadDotHtml));
      const title = item.annotations?.title || item.summary || "未命名会话";
      const wsUri = item.workspaceUris?.[0] || item.workspaces?.[0]?.workspaceFolderAbsoluteUri || "";
      const wsName = wsUri.split("/").filter(Boolean).pop() || "workspace";
      const timeStr = formatRelativeTime(item.lastModifiedTime);

      return `
        <div class="conv-card-wrapper" data-id="${item.id}">
          <div class="conv-card-actions">
            <button class="conv-card-delete-btn" type="button" aria-label="删除会话" data-id="${item.id}">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="3 6 5 6 21 6"></polyline>
                <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path>
                <line x1="10" y1="11" x2="10" y2="17"></line>
                <line x1="14" y1="11" x2="14" y2="17"></line>
              </svg>
              <span>删除</span>
            </button>
          </div>
          <div class="conv-card" data-id="${item.id}" data-title="${escapeHtml(title)}">
            <div class="conv-card-top">
              <div class="conv-title">${escapeHtml(title)}</div>
              ${badgeHtml}
            </div>
            <div class="conv-card-bottom">
              <div class="conv-meta">
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"></path>
                </svg>
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

// --- iOS Conversation List Card Interactions (Swipe-to-Delete & Long-Press Rename) ---

function attachConversationCardInteractions() {
  const wrappers = document.querySelectorAll(".conv-card-wrapper");
  wrappers.forEach((wrapper) => {
    const card = wrapper.querySelector(".conv-card");
    const deleteBtn = wrapper.querySelector(".conv-card-delete-btn");
    const id = wrapper.getAttribute("data-id");
    if (!card) return;

    let startX = 0;
    let startY = 0;
    let currentX = 0;
    let isDragging = false;
    let isHorizontal = null;
    let longPressTimer = null;
    let hasTriggeredLongPress = false;

    const closeCard = () => {
      card.style.transform = "translateX(0)";
      card.classList.remove("swiped");
    };

    const closeOtherCards = () => {
      document.querySelectorAll(".conv-card.swiped").forEach((c) => {
        if (c !== card) {
          c.style.transform = "translateX(0)";
          c.classList.remove("swiped");
        }
      });
    };

    // Touch event handlers
    card.addEventListener("touchstart", (e) => {
      if (e.touches.length > 1) return;
      startX = e.touches[0].clientX;
      startY = e.touches[0].clientY;
      currentX = startX;
      isDragging = false;
      isHorizontal = null;
      hasTriggeredLongPress = false;
      card.classList.remove("swiping");

      // 450ms long press for Rename (aligns with iOS LongPressGesture)
      longPressTimer = setTimeout(() => {
        if (!isDragging && Math.abs(currentX - startX) < 10) {
          hasTriggeredLongPress = true;
          triggerHaptic("medium");
          const title = card.getAttribute("data-title") || "会话";
          openRenameAlert(id, title);
        }
      }, 450);
    }, { passive: true });

    card.addEventListener("touchmove", (e) => {
      currentX = e.touches[0].clientX;
      const currentY = e.touches[0].clientY;
      const dx = currentX - startX;
      const dy = currentY - startY;

      if (isHorizontal === null && (Math.abs(dx) > 6 || Math.abs(dy) > 6)) {
        isHorizontal = Math.abs(dx) > Math.abs(dy);
        if (!isHorizontal || Math.abs(dx) > 8) {
          clearTimeout(longPressTimer);
        }
      }

      if (isHorizontal) {
        clearTimeout(longPressTimer);
        closeOtherCards();
        isDragging = true;
        card.classList.add("swiping");

        const isAlreadySwiped = card.classList.contains("swiped");
        const baseOffset = isAlreadySwiped ? -80 : 0;
        let newX = baseOffset + dx;

        // Clamping & rubber-band resistance
        if (newX > 0) {
          newX = newX * 0.2;
        } else if (newX < -80) {
          newX = -80 + (newX + 80) * 0.25;
        }
        card.style.transform = `translateX(${newX}px)`;
      }
    }, { passive: true });

    card.addEventListener("touchend", () => {
      clearTimeout(longPressTimer);
      card.classList.remove("swiping");

      if (hasTriggeredLongPress) return;

      if (isDragging) {
        const dx = currentX - startX;
        const isAlreadySwiped = card.classList.contains("swiped");
        if (isAlreadySwiped) {
          if (dx > 25) {
            closeCard();
          } else {
            card.style.transform = "translateX(-80px)";
          }
        } else {
          if (dx < -38) {
            card.style.transform = "translateX(-80px)";
            card.classList.add("swiped");
            triggerHaptic("light");
          } else {
            closeCard();
          }
        }
      } else {
        if (card.classList.contains("swiped")) {
          closeCard();
        } else {
          closeOtherCards();
          triggerHaptic("selection");
          navigateTo(`#c=${id}`);
        }
      }
    });

    if (deleteBtn) {
      deleteBtn.addEventListener("click", (e) => {
        e.stopPropagation();
        triggerHaptic("medium");
        openDeleteActionSheet(id);
      });
    }
  });
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
      const convBrightness = 0.85 + progress * 0.15;

      convView.style.transform = `translateX(${convOffset}%)`;
      convView.style.filter = `brightness(${convBrightness})`;
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
      convView.style.transition = "transform 0.25s cubic-bezier(0.32, 0.72, 0, 1), filter 0.25s ease";
      chatView.style.transform = "translateX(100%)";
      convView.style.transform = "translateX(0)";
      convView.style.filter = "brightness(1)";

      triggerHaptic("light");

      setTimeout(() => {
        chatView.style.transition = "";
        convView.style.transition = "";
        chatView.style.transform = "";
        convView.style.transform = "";
        convView.style.filter = "";
        navigateTo("#");
      }, 250);
    } else {
      chatView.style.transition = "transform 0.22s cubic-bezier(0.32, 0.72, 0, 1)";
      convView.style.transition = "transform 0.22s cubic-bezier(0.32, 0.72, 0, 1), filter 0.22s ease";
      chatView.style.transform = "translateX(0)";
      convView.style.transform = "translateX(-28%)";
      convView.style.filter = "brightness(0.85)";

      setTimeout(() => {
        chatView.style.transition = "";
        convView.style.transition = "";
        chatView.style.transform = "";
        convView.style.transform = "";
        convView.style.filter = "";
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

// --- Chat View & Real-Time Stream ---

let activeWs = null;
let wsReconnectTimer = null;
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
  const viewPlanBtn = document.getElementById("btn-view-plan");
  if (proceedBtn) {
    if (canProceed) {
      proceedBtn.classList.remove("hidden");
    } else {
      proceedBtn.classList.add("hidden");
    }
  }
  if (viewPlanBtn) {
    if (canProceed) {
      viewPlanBtn.classList.remove("hidden");
    } else {
      viewPlanBtn.classList.add("hidden");
    }
  }
}

// --- Floating Interaction Card Management ---
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
    selectedInteractionOptionId = interaction.options[0]?.id || "";
  }

  renderInteractionCard();
}

function renderInteractionCard() {
  const container = document.getElementById("interaction-card-container");
  if (!container || !currentPendingInteraction) return;

  const interaction = currentPendingInteraction;
  const isPermission = interaction.type === "permission";
  const selectedOpt = interaction.options.find(o => o.id === selectedInteractionOptionId) || interaction.options[0];
  const isDenyOrWriteIn = selectedOpt?.isDeny || selectedOpt?.id === "no" || selectedOpt?.id?.toLowerCase().includes("deny");

  const iconSvg = isPermission
    ? `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"></path></svg>`
    : `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"></path><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>`;

  container.innerHTML = `
    <div class="interaction-card">
      <div class="interaction-card-header">
        <div class="interaction-icon">${iconSvg}</div>
        <div class="interaction-title-group">
          <div class="interaction-title">${escapeHtml(interaction.title || "需要确认或授权")}</div>
          <div class="interaction-subtitle">${escapeHtml(interaction.subtitle || "等待决策响应")}</div>
        </div>
      </div>

      ${interaction.target ? `
        <div class="interaction-target-box">
          <span class="interaction-target-label">Target</span>
          <span class="interaction-target-path">${escapeHtml(interaction.target)}</span>
        </div>
      ` : ''}

      <div class="interaction-options-list">
        ${interaction.options.map((opt, idx) => {
          const isSelected = opt.id === selectedInteractionOptionId;
          return `
            <div class="interaction-option-item ${isSelected ? 'selected' : ''}" data-opt-id="${escapeHtml(opt.id)}">
              <span class="interaction-option-radio">
                <span class="interaction-radio-dot"></span>
              </span>
              <span class="interaction-option-badge">[${idx + 1}]</span>
              <span class="interaction-option-label">${escapeHtml(opt.label)}</span>
            </div>
          `;
        }).join('')}
      </div>

      <div id="interaction-write-in-wrap" class="interaction-write-in-wrap ${isDenyOrWriteIn ? '' : 'hidden'}">
        <input type="text" id="interaction-write-in-input" class="interaction-write-in-input" placeholder="输入说明或拒绝原因..." />
      </div>

      <div class="interaction-card-actions">
        <button type="button" id="btn-interaction-skip" class="btn-interaction-skip" ${isSubmittingInteraction ? 'disabled' : ''}>Skip</button>
        <button type="button" id="btn-interaction-submit" class="btn-interaction-submit" ${isSubmittingInteraction ? 'disabled' : ''}>
          <span>${isSubmittingInteraction ? '提交中...' : 'Submit'}</span>
          <span class="interaction-submit-key">↵</span>
        </button>
      </div>
    </div>
  `;

  container.classList.remove("hidden");

  // Add click handlers on option items
  container.querySelectorAll(".interaction-option-item").forEach(item => {
    item.addEventListener("click", () => {
      const optId = item.dataset.optId;
      if (optId && optId !== selectedInteractionOptionId) {
        selectedInteractionOptionId = optId;
        renderInteractionCard();
      }
    });
  });

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
    const writeInInput = document.getElementById("interaction-write-in-input");
    const writeInText = writeInInput ? writeInInput.value.trim() : "";

    const payload = {
      cascadeId: activeCascadeId,
      stepIndex: currentPendingInteraction.stepIndex,
      substepIndex: currentPendingInteraction.substepIndex || 0,
      type: currentPendingInteraction.type,
      optionId: isSkip ? "" : (selectedInteractionOptionId || ""),
      writeInText: isSkip ? "" : writeInText,
      skipped: isSkip
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

function closeActiveWs() {
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

  if (wsUri && wsText) {
    const wsName = wsUri.split("/").filter(Boolean).pop() || "workspace";
    wsText.textContent = `📁 ${wsName}`;
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

function connectStreamWs(cascadeId) {
  closeActiveWs();

  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  let wsUrl = `${proto}//${location.host}/gateway/cascade/stream?cascadeId=${encodeURIComponent(cascadeId)}`;
  const token = localStorage.getItem("agy_device_token");
  if (token) {
    wsUrl += `&auth_token=${encodeURIComponent(token)}`;
  }
  
  try {
    const ws = new WebSocket(wsUrl);
    activeWs = ws;

    ws.onopen = () => {
      // WS successfully established: stop HTTP polling fallback
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

        const isRunning = data.status === "CASCADE_RUN_STATUS_RUNNING";

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

        if (data.queuedMessages) {
          LocalQueueManager.syncFromServer(data.queuedMessages);
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
        wsReconnectTimer = setTimeout(() => {
          if (activeCascadeId === cascadeId) {
            connectStreamWs(cascadeId);
          }
        }, 2500);
      }
    };
  } catch (e) {
    fallbackToHttpPolling(cascadeId);
  }
}

function fallbackToHttpPolling(cascadeId) {
  if (activeCascadeId === cascadeId && !pollTimer) {
    pollTimer = setInterval(() => {
      if (activeCascadeId === cascadeId) {
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
    const isRunning = summary?.status === "CASCADE_RUN_STATUS_RUNNING";
    const wsUri = traj.workspaceUris?.[0] || "";

    const dynamicTitle = traj.annotations?.title || traj.summary;
    if (dynamicTitle && document.getElementById("header-title")) {
      document.getElementById("header-title").textContent = dynamicTitle;
    }

    updateChatControls(isRunning, wsUri);
    LocalQueueManager.init(cascadeId);
    RunningTasksManager.init(cascadeId);
    renderMessages(steps, isRunning);

    fetch(`/gateway/cascade/messages?cascadeId=${encodeURIComponent(cascadeId)}&limit=1`)
      .then(res => res.json())
      .then(info => {
        if (activeCascadeId === cascadeId) {
          if (info.activeModel) {
            syncActiveModel(info.activeModel);
          }
          if (info.queuedMessages) {
            LocalQueueManager.syncFromServer(info.queuedMessages);
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

    if (isRunning && (!activeWs || activeWs.readyState !== WebSocket.OPEN) && !pollTimer) {
      pollTimer = setInterval(() => {
        if (activeCascadeId === cascadeId) {
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
        if (hasMedia) {
          for (const m of s.userInput.media) {
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
          step: s
        });
      }
    } else if (type === "CORTEX_STEP_TYPE_PLANNER_RESPONSE") {
      const p = s.plannerResponse || {};
      const resp = (p.response || "").trim();
      const thinking = (p.thinking || "").trim();

      if (resp) {
        flushBatch();
        items.push({
          type: "agent",
          id: `item-agent-${i}`,
          index: i,
          text: p.response,
          thinking: thinking,
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
    return `a:${thinkLen}:${textLen}:${textLast}`;
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

function generateItemHtml(item, isRunning, isLastItem) {
  if (item.type === "error") {
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
            <span class="badge badge-error">error</span>
            <span class="agent-error-title">执行遇到错误</span>
          </div>
          <div class="agent-error-message">${escapeHtml(item.text)}</div>
        </div>
      </div>
    `;
  }
  if (item.type === "user") {
    let imagesHtml = "";
    if (item.media && item.media.length > 0) {
      imagesHtml = `<div class="user-message-images">` +
        item.media.map(m => {
          const src = m.startsWith("data:") ? m : `data:image/jpeg;base64,${m}`;
          return `<img src="${src}" class="bubble-image" onclick="window.open('${src}')" alt="上传图片" />`;
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
    const bodyHtml = item.text ? getCachedMarkdown(item.text) : '<span style="color:var(--text-muted);">执行中...</span>';
    return `
      <div class="bubble markdown-body">
        ${thoughtHtml}
        <div>${bodyHtml}</div>
      </div>
    `;
  }

  if (item.type === "tools") {
    const isActive = isRunning && isLastItem;
    const count = item.steps.length;
    const toolNamesStr = item.toolNames.join(", ") + (item.toolNames.length > 2 ? "..." : "");

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
          <span class="tool-step-name">⚡ ${escapeHtml(s.name)}</span>
          ${s.detail ? `<span class="tool-step-detail">${escapeHtml(s.detail)}</span>` : ''}
        </div>
        <span class="tool-step-status">${escapeHtml(s.status || "DONE")}</span>
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
              <polyline points="9 18 15 12 9 6"></polyline>
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
    return;
  }

  if (activeCascadeId) {
    sessionStepsCache[activeCascadeId] = { steps, isRunning };
  }

  const items = groupSteps(steps);
  const currentChildIds = new Set(items.map(it => it.id));
  currentChildIds.add("agent-thinking-indicator");

  let hasDOMChanges = false;

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
        const wasOpen = existingEl.querySelector("details")?.open;
        existingEl.setAttribute("data-fp", fp);
        existingEl.className = rowClass;
        existingEl.innerHTML = generateItemHtml(item, isRunning, isLastItem);
        if (wasOpen) {
          const newDetails = existingEl.querySelector("details");
          if (newDetails) newDetails.open = true;
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

  // Standalone Agent Thinking Indicator (shown while awaiting response or continuing plan execution)
  const lastItem = items[items.length - 1];
  const isAwaiting = isRunning && lastItem?.type !== "tools";
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

  async stopTask(stepIndex, taskId) {
    if (!activeCascadeId) return;
    if (!confirm("确定要终止此后台任务吗？")) return;

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
      const desc = escapeHtml(task.toolSummary || task.toolAction || task.toolName || "后台任务");
      const cmd = escapeHtml(task.commandLine || "运行中...");
      const idEsc = escapeHtml(task.id || "");
      return `
        <div class="task-item-row" data-id="${idEsc}" data-step="${task.stepIndex}">
          <div class="task-item-body">
            <span class="task-item-desc">${desc}</span>
            <span class="task-item-cmd" title="${cmd}">${cmd}</span>
          </div>
          <button class="task-stop-btn" onclick="RunningTasksManager.stopTask(${task.stepIndex}, '${idEsc}')" title="终止任务" aria-label="终止任务">
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

  init(cascadeId) {
    if (!cascadeId) return;
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

  syncFromServer(serverQueue) {
    if (!Array.isArray(serverQueue)) return;
    if (serverQueue.length > 0) {
      this.queue = serverQueue.map(item => ({
        id: item.id || `server-${Date.now()}`,
        text: item.text,
        createdAt: item.createdAt || new Date().toISOString()
      }));
    } else if (currentTrajectories[activeCascadeId]?.status !== "CASCADE_RUN_STATUS_RUNNING") {
      this.queue = [];
    }
    this.save();
  },

  enqueue(text) {
    if (!text.trim()) return;
    const item = {
      id: `queue-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`,
      text: text.trim(),
      createdAt: new Date().toISOString()
    };
    this.queue.push(item);
    this.save();
  },

  remove(id) {
    this.queue = this.queue.filter(it => it.id !== id);
    this.save();
    if (activeCascadeId) {
      rpc("DeleteAgentMessage", { messageId: id, recipient: activeCascadeId }).catch(() => {});
    }
  },

  async sendNow(id) {
    const item = this.queue.find(it => it.id === id);
    if (!item || !activeCascadeId) return;
    this.remove(id);

    try {
      if (currentTrajectories[activeCascadeId]) {
        currentTrajectories[activeCascadeId].status = "CASCADE_RUN_STATUS_RUNNING";
      }
      updateChatControls(true, null, false);
      await rpc("SendUserCascadeMessage", {
        cascadeId: activeCascadeId,
        model: activeModel,
        items: [{ text: item.text }],
        deliveryStrategy: 1 // NEXT_INVOCATION
      });
      if (!activeWs || activeWs.readyState !== WebSocket.OPEN) {
        connectStreamWs(activeCascadeId);
      }
    } catch (err) {
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

    listEl.innerHTML = this.queue.map(item => `
      <div class="queued-item-row" data-id="${item.id}">
        <span class="queued-item-text">${escapeHtml(item.text)}</span>
        <div class="queued-actions" data-testid="queued-decorators">
          <button class="queued-icon-btn btn-send-now" onclick="LocalQueueManager.sendNow('${item.id}')" title="立即发送" aria-label="立即发送">
            <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 -960 960 960" fill="currentColor">
              <path d="M665.08-450H180v-60H665.08L437.23-737.85L480-780L780-480L480-180l-42.77-42.15L665.08-450Z"></path>
            </svg>
          </button>
          <button class="queued-icon-btn btn-edit" onclick="LocalQueueManager.edit('${item.id}')" title="编辑" aria-label="编辑">
            <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 -960 960 960" fill="currentColor">
              <path d="M200-200h50.46L659.92-609.46l-50.46-50.46L200-250.46V-200Zm-60,60V-275.38L667.62-802.77q9.07-8.24 20.04-12.74T710.65-820t23.31,4.27t19.97,13.58l48.85,49.46q9.31,8.69 13.27,20T820-710.07q0,12.07-4.12,23.03T802.77-667L275.38-140H140ZM760.38-710.15l-50.23-50.23l50.23,50.23Zm-126.13,75.9l-24.79-25.67l50.46,50.46l-25.67-24.79Z"></path>
            </svg>
          </button>
          <button class="queued-icon-btn btn-delete" onclick="LocalQueueManager.remove('${item.id}')" title="删除" aria-label="删除">
            <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 -960 960 960" fill="currentColor">
              <path d="M292.31-140q-29.92,0-51.11-21.19T220-212.31V-720H180v-60H360v-35.38H600V-780H780v60H740v507.69Q740-182 719-161t-51.31,21H292.31ZM680-720H280v507.69q0,5.39 3.46,8.85t8.85,3.46H667.69q4.62,0 8.46-3.85t3.85-8.46V-720ZM376.16-280h60V-640h-60v360Zm147.69,0h60V-640h-60v360ZM280-720v507.69q0,5.39 0,8.85t0,3.46q0,0 0-3.46t0-8.85V-720Z"></path>
            </svg>
          </button>
        </div>
      </div>
    `).join("");
  }
};

// --- Send Message & Actions ---

let isSendingMessage = false;

async function sendMessage() {
  if (!activeCascadeId || isSendingMessage) return;

  const inputEl = document.getElementById("chat-input");
  const text = inputEl.value.trim();
  const hasImages = pendingImages.length > 0;
  if (!text && !hasImages) return;

  isSendingMessage = true;
  const isRunning = currentTrajectories[activeCascadeId]?.status === "CASCADE_RUN_STATUS_RUNNING";

  const imagesToSend = [...pendingImages];
  pendingImages = [];
  renderImagePreviews();

  inputEl.value = "";
  inputEl.style.height = "auto";
  currentCanProceed = false;
  updateProceedButton(false);

  const items = text ? [{ text }] : [];
  const imagesPayload = imagesToSend.map(img => ({
    base64Data: img.base64Data,
    mimeType: img.mimeType || "image/jpeg"
  }));
  const mediaPayload = imagesToSend.map(img => ({
    inlineData: img.base64Data,
    mimeType: img.mimeType || "image/jpeg"
  }));

  if (isRunning) {
    // Enqueue message while agent is running
    LocalQueueManager.enqueue(text || (imagesToSend.length ? `[${imagesToSend.length} 张图片]` : ""));
    updateChatControls(true, null, false);
    try {
      const payload = {
        cascadeId: activeCascadeId,
        model: activeModel,
        items: items,
        deliveryStrategy: 2 // WHEN_IDLE
      };
      if (imagesPayload.length > 0) {
        payload.images = imagesPayload;
        payload.media = mediaPayload;
      }
      await rpc("SendUserCascadeMessage", payload);
    } catch (err) {
      console.warn("[Queue] SendUserCascadeMessage with WHEN_IDLE notification:", err);
    } finally {
      isSendingMessage = false;
    }
    return;
  }

  const streamEl = document.getElementById("messages-stream");
  const tempId = `temp-user-${Date.now()}`;
  let imgHtml = "";
  if (imagesToSend.length > 0) {
    imgHtml = `<div class="user-message-images">` +
      imagesToSend.map(img => `<img src="${img.dataUrl}" class="bubble-image" onclick="window.open('${img.dataUrl}')" alt="上传图片" />`).join("") +
      `</div>`;
  }
  const textHtml = text ? `<div>${escapeHtml(text)}</div>` : "";
  streamEl.insertAdjacentHTML("beforeend", `
    <div id="${tempId}" class="message-row user">
      <div class="bubble">${imgHtml}${textHtml}</div>
    </div>
  `);
  userIsNearBottom = true;
  streamEl.scrollTop = streamEl.scrollHeight;

  try {
    if (currentTrajectories[activeCascadeId]) {
      currentTrajectories[activeCascadeId].status = "CASCADE_RUN_STATUS_RUNNING";
      currentTrajectories[activeCascadeId].needsInput = false;
    }
    updateChatControls(true, null, false);

    const payload = {
      cascadeId: activeCascadeId,
      model: activeModel,
      items: items
    };
    if (imagesPayload.length > 0) {
      payload.images = imagesPayload;
      payload.media = mediaPayload;
    }

    await rpc("SendUserCascadeMessage", payload);

    // Ensure WebSocket stream is actively connected
    if (!activeWs || activeWs.readyState !== WebSocket.OPEN) {
      connectStreamWs(activeCascadeId);
    }
  } catch (err) {
    alert("发送失败: " + err.message);
    const tempEl = document.getElementById(tempId);
    if (tempEl) tempEl.remove();
    inputEl.value = text;
    pendingImages = imagesToSend;
    renderImagePreviews();
  } finally {
    isSendingMessage = false;
  }
}

async function handleProceed() {
  if (!currentCanProceed || !currentProceedArtifactUri || !activeCascadeId) return;
  const artifactUri = currentProceedArtifactUri;
  updateProceedButton(false);
  currentCanProceed = false;

  const streamEl = document.getElementById("messages-stream");
  let thinkingIndicator = document.getElementById("agent-thinking-indicator");
  if (!thinkingIndicator) {
    thinkingIndicator = document.createElement("div");
    thinkingIndicator.id = "agent-thinking-indicator";
    thinkingIndicator.className = "agent-thinking-card message-entering";
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
  }
  userIsNearBottom = true;
  streamEl.scrollTop = streamEl.scrollHeight;

  try {
    if (currentTrajectories[activeCascadeId]) {
      currentTrajectories[activeCascadeId].status = "CASCADE_RUN_STATUS_RUNNING";
      currentTrajectories[activeCascadeId].needsInput = false;
    }
    updateChatControls(true, null, false);

    await rpc("SendUserCascadeMessage", {
      cascadeId: activeCascadeId,
      model: activeModel,
      items: [],
      artifactComments: [
        {
          artifactUri: artifactUri,
          scope: { case: "fullFile", value: {} },
          approvalStatus: 1,
          comment: ""
        }
      ]
    });

    if (!activeWs || activeWs.readyState !== WebSocket.OPEN) {
      connectStreamWs(activeCascadeId);
    }
  } catch (err) {
    alert("确认方案失败: " + err.message);
    if (thinkingIndicator) thinkingIndicator.remove();
    updateProceedButton(true);
    currentCanProceed = true;
    updateChatControls(false, null, false);
  }
}

async function cancelCurrentTask() {
  if (!activeCascadeId) return;
  if (!confirm("确定要终止当前 Agent 任务吗？")) return;
  currentCanProceed = false;
  updateProceedButton(false);
  updatePendingInteraction(null, false);

  try {
    await rpc("CancelCascadeInvocation", { cascadeId: activeCascadeId });
    if (currentTrajectories[activeCascadeId]) {
      currentTrajectories[activeCascadeId].status = "CASCADE_RUN_STATUS_IDLE";
    }
    updateChatControls(false);
  } catch (err) {
    alert("取消任务失败: " + err.message);
  }
}

// --- Markdown File Viewer Sheet ---
let currentViewerData = null;
let currentViewerUri = null;

async function fetchFileContent(uri, cascadeId) {
  const params = new URLSearchParams();
  if (uri) params.set("uri", uri);
  if (cascadeId) params.set("cascade_id", cascadeId);
  const resp = await fetch(`/api/v1/files/content?${params.toString()}`);
  if (!resp.ok) {
    let msg = `HTTP ${resp.status}`;
    try {
      const err = await resp.json();
      if (err.error) msg = err.error;
    } catch (_) {}
    throw new Error(msg);
  }
  return resp.json();
}

async function openMarkdownViewer(uri, title) {
  const sheet = document.getElementById("sheet-markdown-viewer");
  if (!sheet) return;

  currentViewerUri = uri || "implementation_plan.md";
  currentViewerData = null;

  const titleEl = document.getElementById("md-viewer-title");
  const subtitleEl = document.getElementById("md-viewer-subtitle");
  const loadingEl = document.getElementById("md-viewer-loading");
  const errorEl = document.getElementById("md-viewer-error");
  const contentEl = document.getElementById("md-viewer-content");
  const proceedBar = document.getElementById("md-viewer-proceed-bar");

  // Determine display title & subtitle
  let displayTitle = "Implementation Plan";
  let displaySubtitle = uri || "implementation_plan.md";
  const filename = (uri || "").split("/").pop().split("?")[0] || uri;
  displaySubtitle = filename || displaySubtitle;

  if (titleEl) titleEl.textContent = displayTitle;
  if (subtitleEl) subtitleEl.textContent = displaySubtitle;

  // Reset state
  if (contentEl) contentEl.innerHTML = "";
  if (errorEl) errorEl.classList.add("hidden");
  if (loadingEl) loadingEl.classList.remove("hidden");

  // Initial proceed bar check
  const isPlan = filename.includes("implementation_plan") || (title && title.includes("实施方案"));
  if (proceedBar) {
    if (currentCanProceed && isPlan) {
      proceedBar.classList.remove("hidden");
    } else {
      proceedBar.classList.add("hidden");
    }
  }

  sheet.classList.remove("hidden");
  triggerHaptic("selection");

  try {
    const data = await fetchFileContent(currentViewerUri, activeCascadeId);
    currentViewerData = data;

    if (loadingEl) loadingEl.classList.add("hidden");

    if (data.filename && subtitleEl) {
      subtitleEl.textContent = data.filename;
    }

    // Render markdown content using chat's rich markdown parser
    if (contentEl) {
      contentEl.innerHTML = renderMarkdown(data.content || "");
    }

    // Check proceed capability
    if (proceedBar) {
      const canProceedThis = currentCanProceed && (data.request_feedback || isPlan);
      if (canProceedThis) {
        proceedBar.classList.remove("hidden");
      } else {
        proceedBar.classList.add("hidden");
      }
    }
  } catch (err) {
    if (loadingEl) loadingEl.classList.add("hidden");
    if (errorEl) {
      errorEl.classList.remove("hidden");
      const errText = document.getElementById("md-viewer-error-text");
      if (errText) errText.textContent = `加载失败: ${err.message}`;
    }
  }
}

function closeMarkdownViewer() {
  const sheet = document.getElementById("sheet-markdown-viewer");
  if (sheet) {
    sheet.classList.add("hidden");
  }
  currentViewerData = null;
}

window.openMarkdownViewer = openMarkdownViewer;
window.closeMarkdownViewer = closeMarkdownViewer;

function initMarkdownViewer() {
  const sheet = document.getElementById("sheet-markdown-viewer");
  const closeBtn = document.getElementById("btn-md-viewer-close");
  const retryBtn = document.getElementById("btn-md-viewer-retry");
  const proceedBtn = document.getElementById("btn-md-viewer-proceed");
  const viewPlanBtn = document.getElementById("btn-view-plan");

  closeBtn?.addEventListener("click", closeMarkdownViewer);

  sheet?.addEventListener("click", (e) => {
    if (e.target === sheet) {
      closeMarkdownViewer();
    }
  });

  retryBtn?.addEventListener("click", () => {
    if (currentViewerUri) {
      openMarkdownViewer(currentViewerUri);
    }
  });

  proceedBtn?.addEventListener("click", () => {
    closeMarkdownViewer();
    handleProceed();
  });

  viewPlanBtn?.addEventListener("click", () => {
    openMarkdownViewer("implementation_plan.md", "Implementation Plan");
  });

  // Delegated click on document for any markdown file links
  document.addEventListener("click", (e) => {
    const link = e.target.closest("a");
    if (!link) return;

    const dataMdUrl = link.getAttribute("data-md-url");
    const href = link.getAttribute("href") || "";

    let isLocalMd = !!dataMdUrl || link.classList.contains("markdown-file-link");
    if (!isLocalMd) {
      const lower = href.toLowerCase();
      const isHttp = lower.startsWith("http://") || lower.startsWith("https://");
      const isExternal = isHttp && !lower.includes(window.location.host);
      if (!isExternal) {
        if (lower.endsWith(".md") || lower.endsWith(".markdown") || lower.includes("/brain/") || lower.includes("/static/artifacts/")) {
          isLocalMd = true;
        }
      }
    }

    if (isLocalMd) {
      e.preventDefault();
      e.stopPropagation();
      const targetUrl = dataMdUrl || href;
      const targetTitle = link.getAttribute("data-md-title") || link.textContent.trim() || "Markdown 文档";
      openMarkdownViewer(targetUrl, targetTitle);
    }
  });
}

// --- iOS Bottom Sheets (New Conversation & Settings) ---

let discoveredProjects = [];

async function openNewSheet() {
  const sheet = document.getElementById("sheet-new");
  if (!sheet) return;
  sheet.classList.remove("hidden");

  // 1. Fetch discovered upstream projects
  const wsSelect = document.getElementById("new-workspace-select");
  const wsInput = document.getElementById("new-workspace");

  try {
    const res = await fetch("/gateway/projects");
    if (res.ok) {
      discoveredProjects = await res.json();
      if (discoveredProjects && discoveredProjects.length > 0) {
        wsSelect.innerHTML = `<option value="">-- 请选择目标项目 (${discoveredProjects.length} 个可用) --</option>` +
          discoveredProjects.map(p => {
            const countStr = p.sessionCount > 0 ? ` (${p.sessionCount}个会话)` : "";
            const wsTag = p.isWorkspace ? " [工作区]" : "";
            return `<option value="${escapeHtml(p.path)}">${escapeHtml(p.name)}${wsTag}${countStr}</option>`;
          }).join("");
        
        // Auto-select first project if input is empty
        if (!wsInput.value && discoveredProjects[0]) {
          wsSelect.value = discoveredProjects[0].path;
          wsInput.value = discoveredProjects[0].path;
        }
      } else {
        wsSelect.innerHTML = `<option value="">(未探测到项目，请在下方手动输入)</option>`;
      }
    }
  } catch (_) {
    wsSelect.innerHTML = `<option value="">(无法获取项目列表，可手动输入)</option>`;
  }

  wsSelect.onchange = () => {
    if (wsSelect.value) {
      wsInput.value = wsSelect.value;
    }
  };

  // 2. Fetch available models
  const modelSelect = document.getElementById("new-model");
  if (availableModels.length === 0) {
    try {
      const data = await rpc("GetAvailableModels");
      const models = data.response?.models || {};
      availableModels = Object.entries(models).map(([id, m]) => ({
        id,
        name: m.displayName || id
      }));

      modelSelect.innerHTML = `<option value="">自动推荐模型</option>` +
        availableModels.map(m => `<option value="${escapeHtml(m.id)}">${escapeHtml(m.name)}</option>`).join("");
    } catch (_) {}
  }

  if (modelSelect && activeModel) {
    let matched = false;
    for (const opt of modelSelect.options) {
      if (opt.value === activeModel || 
          (activeModel.includes("claude") && (opt.value.toLowerCase().includes("claude") || opt.value.includes("m26"))) ||
          (activeModel.includes("gemini") && (opt.value.toLowerCase().includes("gemini") || opt.value.includes("m318")))) {
        modelSelect.value = opt.value;
        matched = true;
        break;
      }
    }
    if (!matched && activeModel) {
      modelSelect.value = activeModel;
    }
  }
}

function closeNewSheet() {
  const sheet = document.getElementById("sheet-new");
  if (sheet) sheet.classList.add("hidden");
}

function updateAuthUI() {
  const statusEl = document.getElementById("settings-auth-status");
  const deviceIdRow = document.getElementById("settings-device-id-row");
  const deviceIdEl = document.getElementById("settings-device-id");
  const btnUnpair = document.getElementById("btn-unpair-device");
  const token = localStorage.getItem("agy_device_token");
  const devId = localStorage.getItem("agy_device_id");

  if (token) {
    if (statusEl) {
      statusEl.textContent = "已配对";
      statusEl.className = "status-badge connected";
    }
    if (deviceIdRow) deviceIdRow.classList.remove("hidden");
    if (deviceIdEl) deviceIdEl.textContent = devId || "已绑定";
    if (btnUnpair) btnUnpair.classList.remove("hidden");
  } else {
    if (statusEl) {
      statusEl.textContent = "未配对";
      statusEl.className = "status-badge disconnected";
    }
    if (deviceIdRow) deviceIdRow.classList.add("hidden");
    if (btnUnpair) btnUnpair.classList.add("hidden");
  }
}

function parsePairingInput(raw) {
  raw = (raw || "").trim();
  if (!raw) return null;

  // Case 1: agy://pair?host=...&port=...&code=...
  if (raw.startsWith("agy://pair")) {
    try {
      const url = new URL(raw.replace("agy://", "http://"));
      const code = url.searchParams.get("code");
      if (code) return code;
    } catch (_) {
      const match = raw.match(/code=([a-zA-Z0-9]+)/);
      if (match) return match[1];
    }
  }

  return raw;
}

async function pairWithCode(code) {
  const resp = await originalFetch("/api/v1/auth/pair", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      pairing_code: code,
      device_name: `Web Browser (${navigator.userAgent.includes("iPhone") ? "iPhone Safari" : "Desktop/PWA"})`,
      platform: "pwa"
    })
  });

  const data = await resp.json();
  if (!resp.ok) {
    throw new Error(data.error || `配对失败 (HTTP ${resp.status})`);
  }

  localStorage.setItem("agy_device_token", data.device_token);
  localStorage.setItem("agy_device_id", data.device_id);
  updateAuthUI();
  return data;
}

function openPairingSheet(errorMsg = "") {
  const sheet = document.getElementById("sheet-pairing");
  const errEl = document.getElementById("pairing-error-msg");
  const inputEl = document.getElementById("input-pairing-code");
  if (errEl) {
    if (errorMsg) {
      errEl.textContent = errorMsg;
      errEl.classList.remove("hidden");
    } else {
      errEl.textContent = "";
      errEl.classList.add("hidden");
    }
  }
  if (inputEl) {
    inputEl.value = "";
    setTimeout(() => inputEl.focus(), 150);
  }
  if (sheet) sheet.classList.remove("hidden");
}

function closePairingSheet() {
  const sheet = document.getElementById("sheet-pairing");
  if (sheet) sheet.classList.add("hidden");
}

async function submitPairing() {
  const inputEl = document.getElementById("input-pairing-code");
  const errEl = document.getElementById("pairing-error-msg");
  const submitBtn = document.getElementById("btn-sheet-pairing-submit");
  const raw = inputEl ? inputEl.value : "";
  const code = parsePairingInput(raw);

  if (!code) {
    if (errEl) {
      errEl.textContent = "请输入有效的配对码或配对链接";
      errEl.classList.remove("hidden");
    }
    return;
  }

  if (submitBtn) submitBtn.disabled = true;
  if (errEl) errEl.classList.add("hidden");

  try {
    await pairWithCode(code);
    closePairingSheet();
    loadConversations();
    checkGatewayStatus();
  } catch (err) {
    if (errEl) {
      errEl.textContent = err.message || "配对失败，请检查配对码是否过期或失效";
      errEl.classList.remove("hidden");
    }
  } finally {
    if (submitBtn) submitBtn.disabled = false;
  }
}

function unpairDevice() {
  if (confirm("确定要解除当前设备的配对绑定吗？")) {
    localStorage.removeItem("agy_device_token");
    localStorage.removeItem("agy_device_id");
    updateAuthUI();
    loadConversations();
  }
}

function openSettingsSheet() {
  checkGatewayStatus();
  updateAuthUI();
  const sheet = document.getElementById("sheet-settings");
  if (sheet) sheet.classList.remove("hidden");
}

function closeSettingsSheet() {
  const sheet = document.getElementById("sheet-settings");
  if (sheet) sheet.classList.add("hidden");
}

async function createConversation() {
  const ws = document.getElementById("new-workspace").value.trim();
  const model = document.getElementById("new-model")?.value || activeModel;
  const prompt = document.getElementById("new-prompt").value.trim();

  if (!ws) {
    alert("请选择或输入目标工作区路径");
    return;
  }

  if (!prompt) {
    alert("请输入首条指令");
    return;
  }

  const createBtn = document.getElementById("btn-sheet-new-create");
  if (createBtn) {
    createBtn.textContent = "创建中...";
    createBtn.disabled = true;
  }

  try {
    const res = await fetch("/gateway/cascade/new", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        workspaceUri: ws,
        prompt: prompt,
        model: model || undefined
      })
    });

    const data = await res.json();
    if (!res.ok || data.status === "error" || !data.cascadeId) {
      throw new Error(data.error || "创建会话失败");
    }

    const cascadeId = data.cascadeId;
    closeNewSheet();
    document.getElementById("new-prompt").value = "";
    navigateTo("#c=" + cascadeId);
    await loadConversations();
  } catch (err) {
    alert("创建会话失败: " + err.message);
  } finally {
    if (createBtn) {
      createBtn.textContent = "开始执行";
      createBtn.disabled = false;
    }
  }
}

// --- Helpers ---

/** Validates that a URL uses a safe protocol scheme. Blocks javascript:, data:, vbscript: etc. */
function isSafeURL(url) {
  if (!url) return false;
  const trimmed = url.replace(/^[\s\u00A0]+/, "").toLowerCase();
  // Allow relative URLs, anchors, and protocol-relative URLs
  if (trimmed.startsWith("/") || trimmed.startsWith("#") || trimmed.startsWith("./") || trimmed.startsWith("../")) return true;
  // Allow only safe protocols
  const safeProtocols = ["http:", "https:", "file:", "mailto:"];
  for (const proto of safeProtocols) {
    if (trimmed.startsWith(proto)) return true;
  }
  // Block if it looks like a protocol (contains ":" before any "/")
  const colonIdx = trimmed.indexOf(":");
  if (colonIdx > 0 && colonIdx < trimmed.indexOf("/")) return false;
  // Allow bare URLs without protocol (e.g. "example.com/path")
  return colonIdx === -1;
}

function escapeHtml(str) {
  if (!str) return "";
  return String(str)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#039;");
}

function formatRelativeTime(dateStr) {
  if (!dateStr) return "";
  const diff = (Date.now() - new Date(dateStr).getTime()) / 1000;
  if (diff < 60) return "刚刚";
  if (diff < 3600) return `${Math.floor(diff / 60)}分钟前`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}小时前`;
  return `${Math.floor(diff / 86400)}天前`;
}

let fileIconTheme = null;
fetch("/icons/symbol-icon-theme.json")
  .then(res => res.json())
  .then(data => { fileIconTheme = data; })
  .catch(err => console.warn("Failed to load file icon theme:", err));

const fileIconFallback = {
  "go.mod": "go-mod", "go.sum": "go-mod", "package.json": "node",
  "package-lock.json": "node", "dockerfile": "docker", "makefile": "shell",
  "info.plist": "xml", "readme.md": "markdown",
  "go": "go", "swift": "swift", "html": "code-orange", "htm": "code-orange",
  "json": "brackets-yellow", "md": "markdown", "plist": "xml", "xml": "xml",
  "py": "python", "js": "js", "ts": "ts", "jsx": "react", "tsx": "react",
  "css": "sass", "scss": "sass", "sh": "shell", "bash": "shell", "zsh": "shell",
  "yaml": "yaml", "yml": "yaml", "sql": "database", "rs": "rust", "c": "c",
  "cpp": "cplus", "java": "java", "kt": "kotlin", "png": "image", "jpg": "image"
};

function resolveFileIcon(nameOrUrl) {
  if (!nameOrUrl) return null;
  let clean = nameOrUrl.trim().replace(/^file:\/\//, "");
  let filename = clean.split("/").pop().toLowerCase();
  
  if (fileIconTheme) {
    if (fileIconTheme.fileNames && fileIconTheme.fileNames[filename]) {
      return fileIconTheme.fileNames[filename];
    }
    let ext = filename.split(".").pop();
    if (ext && fileIconTheme.fileExtensions && fileIconTheme.fileExtensions[ext]) {
      return fileIconTheme.fileExtensions[ext];
    }
  }
  
  if (fileIconFallback[filename]) return fileIconFallback[filename];
  let ext = filename.split(".").pop();
  if (ext && fileIconFallback[ext]) return fileIconFallback[ext];
  return null;
}

const latexSymbolLookup = {
  // Blackboard bold & Mathcal
  "mathbb{R}": "ℝ", "mathbf{R}": "ℝ",
  "mathbb{N}": "ℕ", "mathbf{N}": "ℕ",
  "mathbb{Z}": "ℤ", "mathbf{Z}": "ℤ",
  "mathbb{Q}": "ℚ", "mathbf{Q}": "ℚ",
  "mathbb{C}": "ℂ", "mathbf{C}": "ℂ",
  "mathbb{E}": "𝔼", "mathbb{P}": "ℙ",
  "mathbb{H}": "ℍ", "mathbb{F}": "𝔽",
  "mathcal{L}": "ℒ", "mathcal{O}": "𝒪",
  "mathcal{N}": "𝒩", "mathcal{H}": "ℋ",
  "mathcal{F}": "ℱ", "mathcal{D}": "𝒟",

  // Long arrows
  "longleftrightarrow": "⟷", "Longleftrightarrow": "⟺",
  "longrightarrow": "⟶", "longleftarrow": "⟵",
  "Longrightarrow": "⟹", "Longleftarrow": "⟸",
  "longmapsto": "⟼",

  // Standard arrows & harpoons
  "rightleftharpoons": "⇌", "hookrightarrow": "↪", "hookleftarrow": "↩",
  "rightarrow": "→", "leftarrow": "←",
  "leftrightarrow": "↔", "Rightarrow": "⇒", "Leftarrow": "⇐", "Leftrightarrow": "⇔",
  "to": "→", "gets": "←", "implies": "⇒", "iff": "⇔",
  "uparrow": "↑", "downarrow": "↓", "updownarrow": "↕",
  "Uparrow": "⇑", "Downarrow": "⇓", "Updownarrow": "⇕",
  "nearrow": "↗", "searrow": "↘", "swarrow": "↙", "nwarrow": "↖",
  "mapsto": "↦",

  // Comparisons & Relations
  "leqslant": "≤", "geqslant": "≥", "leq": "≤", "geq": "≥",
  "le": "≤", "ge": "≥", "neq": "≠", "ne": "≠",
  "approx": "≈", "simeq": "≃", "cong": "≅", "equiv": "≡",
  "propto": "∝", "ll": "≪", "gg": "≫", "parallel": "∥", "perp": "⊥",
  "sim": "∼", "subset": "⊂", "supset": "⊃",
  "subseteq": "⊆", "supseteq": "⊇", "subsetneq": "⊊", "supsetneq": "⊋",
  "notin": "∉", "in": "∈", "cup": "∪", "cap": "∩", "setminus": "∖",
  "emptyset": "∅", "empty": "∅", "forall": "∀", "exists": "∃",

  // Operators & Calculus
  "times": "×", "div": "÷", "pm": "±", "mp": "∓",
  "cdot": "·", "cdots": "⋯", "ldots": "…", "vdots": "⋮", "ddots": "⋱",
  "bullet": "•", "circ": "∘", "star": "⋆", "ast": "∗",
  "oplus": "⊕", "ominus": "⊖", "otimes": "⊗", "odot": "⊙",
  "iiint": "∭", "iint": "∬", "oint": "∮", "int": "∫",
  "sum": "∑", "prod": "∏", "partial": "∂", "nabla": "∇", "infty": "∞", "sqrt": "√",
  "degree": "°",

  // Greek capital letters
  "Gamma": "Γ", "Delta": "Δ", "Theta": "Θ", "Lambda": "Λ", "Xi": "Ξ",
  "Pi": "Π", "Sigma": "Σ", "Upsilon": "Υ", "Phi": "Φ", "Psi": "Ψ", "Omega": "Ω",

  // Greek lowercase letters
  "alpha": "α", "beta": "β", "gamma": "γ", "delta": "δ",
  "varepsilon": "ε", "epsilon": "ϵ", "zeta": "ζ", "eta": "η",
  "vartheta": "ϑ", "theta": "θ", "iota": "ι", "kappa": "κ",
  "lambda": "λ", "mu": "μ", "nu": "ν", "xi": "ξ", "pi": "π",
  "varrho": "ϱ", "rho": "ρ", "varsigma": "ς", "sigma": "σ",
  "tau": "τ", "upsilon": "υ", "varphi": "φ", "phi": "ϕ",
  "chi": "χ", "psi": "ψ", "omega": "ω"
};

const wordBoundaryCommands = new Set(["to", "in", "le", "ge", "ne", "empty", "gets", "sim"]);

function replaceSymbolsSinglePass(str) {
  if (!str || !str.includes("\\")) return str;
  let out = "";
  let i = 0;
  const len = str.length;

  while (i < len) {
    if (str[i] === "\\") {
      let j = i + 1;
      while (j < len && ((str.charCodeAt(j) >= 65 && str.charCodeAt(j) <= 90) || (str.charCodeAt(j) >= 97 && str.charCodeAt(j) <= 122))) {
        j++;
      }
      const cmd = str.slice(i + 1, j);
      let fullKey = cmd;
      let nextJ = j;

      if (j < len && str[j] === "{") {
        const closeIdx = str.indexOf("}", j + 1);
        if (closeIdx !== -1 && closeIdx - j <= 12) {
          const paramCandidate = cmd + "{" + str.slice(j + 1, closeIdx) + "}";
          if (latexSymbolLookup[paramCandidate]) {
            fullKey = paramCandidate;
            nextJ = closeIdx + 1;
          }
        }
      }

      if (fullKey && latexSymbolLookup[fullKey]) {
        // Word boundary check: if command is short, next char cannot be ASCII letter
        if (wordBoundaryCommands.has(fullKey) && nextJ < len &&
            ((str.charCodeAt(nextJ) >= 65 && str.charCodeAt(nextJ) <= 90) || (str.charCodeAt(nextJ) >= 97 && str.charCodeAt(nextJ) <= 122))) {
          out += str[i];
          i++;
        } else {
          out += latexSymbolLookup[fullKey];
          i = nextJ;
        }
      } else {
        out += str[i];
        i++;
      }
    } else {
      out += str[i];
      i++;
    }
  }
  return out;
}

const supMap = { "0": "⁰", "1": "¹", "2": "²", "3": "³", "4": "⁴", "5": "⁵", "6": "⁶", "7": "⁷", "8": "⁸", "9": "⁹", "+": "⁺", "-": "⁻", "=": "⁼", "(": "⁽", ")": "⁾", "n": "ⁿ", "i": "ⁱ", "j": "ʲ", "a": "ᵃ", "b": "ᵇ", "c": "ᶜ", "d": "ᵈ", "e": "ᵉ", "f": "ᶠ", "g": "ᵍ", "h": "ʰ", "k": "ᵏ", "l": "ˡ", "m": "ᵐ", "o": "ᵒ", "p": "ᵖ", "r": "ʳ", "s": "ˢ", "t": "ᵗ", "u": "ᵘ", "v": "ᵛ", "w": "ʷ", "x": "ˣ", "y": "ʸ", "z": "ᶻ", "T": "ᵀ" };
const subMap = { "0": "₀", "1": "₁", "2": "₂", "3": "₃", "4": "₄", "5": "₅", "6": "₆", "7": "₇", "8": "₈", "9": "₉", "+": "₊", "-": "₋", "=": "₌", "(": "₍", ")": "₎", "a": "ₐ", "e": "ₑ", "h": "ₕ", "i": "ᵢ", "j": "ⱼ", "k": "ₖ", "l": "ₗ", "m": "ₘ", "n": "ₙ", "o": "ₒ", "p": "ₚ", "r": "ᵣ", "s": "ₛ", "t": "ₜ", "u": "ᵤ", "v": "ᵥ", "x": "ₓ" };

function cleanMathExpr(str) {
  if (!str) return "";
  str = str.replace(/\\(?:text|mathrm|mathbf|mathit|operatorname)\{([^}]*)\}/g, "$1");
  str = str.replace(/\\frac\{([^}]*)\}\{([^}]*)\}/g, "$1 / $2");
  str = str.replace(/\\sqrt\{([^}]*)\}/g, "√($1)");
  str = str.replace(/\\left\(/g, "(").replace(/\\right\)/g, ")");
  str = str.replace(/\\left\[/g, "[").replace(/\\right\]/g, "]");
  str = str.replace(/\\left\\\{/g, "{").replace(/\\right\\\}/g, "}");
  str = str.replace(/\\\{/g, "{").replace(/\\\}/g, "}");
  str = str.replace(/\\%/g, "%").replace(/\\_/g, "_").replace(/\\&/g, "&");
  str = str.replace(/\\,/g, " ").replace(/\\;/g, " ").replace(/\\quad/g, " ").replace(/\\qquad/g, "  ");
  str = replaceSymbolsSinglePass(str);
  str = str.replace(/\^\{([0-9a-zA-Z\+\-\=\(\)]+)\}/g, (_, chars) => chars.split("").map(c => supMap[c] || c).join(""));
  str = str.replace(/\^([0-9a-zA-Z\+\-\*])/g, (_, c) => supMap[c] || c);
  str = str.replace(/_\{([0-9a-zA-Z\+\-\=\(\)]+)\}/g, (_, chars) => chars.split("").map(c => subMap[c] || c).join(""));
  str = str.replace(/_([0-9a-zA-Z])/g, (_, c) => subMap[c] || c);
  return str.trim();
}

function processMathSymbols(text) {
  if (!text) return "";
  // Fast-path: if text does not contain backslash or dollar sign, no LaTeX math can be present
  if (!text.includes("\\") && !text.includes("$")) return text;

  const codeBlocks = [];
  text = text.replace(/```[a-zA-Z0-9_-]*\n[\s\S]*?```/g, m => {
    codeBlocks.push(m);
    return `XXAGYBLOCKTOKEN${codeBlocks.length - 1}XX`;
  });
  const inlineCodes = [];
  text = text.replace(/`[^`\n]+`/g, m => {
    inlineCodes.push(m);
    return `XXAGYINLINETOKEN${inlineCodes.length - 1}XX`;
  });

  text = text.replace(/\$\$([\s\S]*?)\$\$/g, (_, m) => cleanMathExpr(m));
  text = text.replace(/\\\[([\s\S]*?)\\\]/g, (_, m) => cleanMathExpr(m));
  text = text.replace(/(?<!\\)\$(?!\s)([^$\n]+?)(?<!\s)(?<!\\)\$/g, (_, m) => cleanMathExpr(m));
  text = text.replace(/\\\(([\s\S]*?)\\\)/g, (_, m) => cleanMathExpr(m));

  // Single-pass replacement for standalone LaTeX commands in prose
  text = replaceSymbolsSinglePass(text);

  inlineCodes.forEach((c, idx) => {
    text = text.replace(`XXAGYINLINETOKEN${idx}XX`, c);
  });
  codeBlocks.forEach((c, idx) => {
    text = text.replace(`XXAGYBLOCKTOKEN${idx}XX`, c);
  });
  return text;
}

function renderInlineMarkdown(text) {
  if (!text) return "";
  if (text.includes("implementation_plan.md") && !text.includes("[implementation_plan.md]") && !text.includes("](implementation_plan.md)")) {
    text = text.replace(/implementation_plan\.md/g, "[implementation_plan.md](implementation_plan.md)");
  }
  if (text.includes("walkthrough.md") && !text.includes("[walkthrough.md]") && !text.includes("](walkthrough.md)")) {
    text = text.replace(/walkthrough\.md/g, "[walkthrough.md](walkthrough.md)");
  }
  if (text.includes("task.md") && !text.includes("[task.md]") && !text.includes("](task.md)")) {
    text = text.replace(/task\.md/g, "[task.md](task.md)");
  }

  let html = escapeHtml(text);

  // Images (only allow safe URL protocols)
  html = html.replace(/!\[([^\]]*)\]\(([^)]+)\)/g, (_, alt, url) => {
    if (!isSafeURL(url)) return escapeHtml(`![${alt}](${url})`);
    return `<img src="${url}" alt="${alt}" class="markdown-image" />`;
  });

  // Markdown links with file icon support (only allow safe URL protocols)
  html = html.replace(/(?<!\!)\[([^\]]+)\]\(([^)]+)\)/g, (_, linkText, url) => {
    if (!isSafeURL(url)) return `${linkText}`;
    const icon = resolveFileIcon(linkText) || resolveFileIcon(url);
    const lower = url.toLowerCase();
    const isMd = lower.endsWith(".md") || lower.endsWith(".markdown") || lower.includes("/brain/") || lower.includes("implementation_plan");
    const extraClass = isMd ? " markdown-file-link" : "";
    if (icon) {
      return `<a href="${url}" class="file-link${extraClass}" data-md-url="${url}" data-md-title="${escapeHtml(linkText)}"><img src="/icons/files/${icon}.svg" class="file-icon" alt="" /><span>${linkText}</span></a>`;
    }
    return `<a href="${url}" class="text-link${extraClass}" data-md-url="${url}" data-md-title="${escapeHtml(linkText)}">${linkText}</a>`;
  });

  // Inline code (e.g. `foo`)
  html = html.replace(/`([^`]+)`/g, '<code class="inline-code">$1</code>');

  // Bold & Italic
  html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
  html = html.replace(/__([^_]+)__/g, '<strong>$1</strong>');
  html = html.replace(/(?<!\*)\*([^*]+)\*(?!\*)/g, '<em>$1</em>');
  html = html.replace(/(?<!_)_([^_]+)_(?!_)/g, '<em>$1</em>');

  // Strikethrough
  html = html.replace(/~~([^~]+)~~/g, '<del>$1</del>');

  return html;
}

function renderMarkdown(md) {
  if (!md) return "";
  md = processMathSymbols(md);

  const lines = md.split("\n");
  const blocks = [];
  let i = 0;

  while (i < lines.length) {
    const line = lines[i];
    const trimmed = line.trim();

    if (!trimmed) {
      i++;
      continue;
    }

    // 1. Fenced Code Block
    if (trimmed.startsWith("```")) {
      const lang = trimmed.slice(3).trim();
      const codeLines = [];
      i++;
      while (i < lines.length) {
        if (lines[i].trim().startsWith("```")) {
          i++;
          break;
        }
        codeLines.push(lines[i]);
        i++;
      }
      const langClean = (lang || "").toLowerCase();
      const displayLang = langClean ? langClean.toUpperCase() : "CODE";
      const codeEscaped = escapeHtml(codeLines.join("\n"));
      blocks.push(`
        <div class="code-block-card">
          <div class="code-block-header">
            <span class="code-block-lang">${displayLang}</span>
            <button class="code-copy-btn" onclick="copyCode(this)" type="button" aria-label="复制代码">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
                <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
              </svg>
              <span>复制</span>
            </button>
          </div>
          <pre class="code-block-pre"><code class="lang-${langClean}">${codeEscaped}</code></pre>
        </div>
      `);
      continue;
    }

    // 2. Horizontal Divider
    if (trimmed === "---" || trimmed === "***" || trimmed === "___") {
      blocks.push(`<hr class="ios-divider" />`);
      i++;
      continue;
    }

    // 3. Headings (# H1..H6)
    if (trimmed.startsWith("#")) {
      let level = 0;
      while (level < trimmed.length && trimmed[level] === "#") {
        level++;
      }
      if (level <= 6 && trimmed.length > level && trimmed[level] === " ") {
        const hText = trimmed.slice(level + 1).trim();
        blocks.push(`<h${level}>${renderInlineMarkdown(hText)}</h${level}>`);
        i++;
        continue;
      }
    }

    // 4. Tables (| Header | Header |)
    if (trimmed.startsWith("|") && trimmed.endsWith("|") && trimmed.includes("|")) {
      const tableLines = [];
      while (i < lines.length) {
        const tLine = lines[i].trim();
        if (tLine.startsWith("|") && tLine.endsWith("|")) {
          tableLines.push(tLine);
          i++;
        } else {
          break;
        }
      }
      if (tableLines.length >= 2) {
        const parseTableRow = (rowStr) => {
          const placeholder = "\uE000";
          const sanitized = rowStr.replace(/\\\|/g, placeholder);
          const parts = sanitized.split("|");
          if (parts.length < 2) return [];
          return parts.slice(1, parts.length - 1).map(c => c.replace(/\uE000/g, "|").trim());
        };
        const headers = parseTableRow(tableLines[0]);
        let alignments = [];
        const rows = [];
        for (let rIdx = 1; rIdx < tableLines.length; rIdx++) {
          const r = parseTableRow(tableLines[rIdx]);
          // Skip separator row (| --- | :--- |)
          const isSep = r.length > 0 && r.every(cell => /^[\s\-:]+$/.test(cell));
          if (isSep) {
            if (alignments.length === 0) {
              alignments = r.map(c => {
                const tr = c.trim();
                const left = tr.startsWith(":");
                const right = tr.endsWith(":");
                if (left && right) return "center";
                if (right) return "right";
                return "left";
              });
            }
            continue;
          }
          rows.push(r);
        }

        const maxCols = Math.max(headers.length, ...rows.map(r => r.length));
        if (maxCols > 0) {
          let tableHtml = `<div class="table-wrapper"><table class="ios-markdown-table"><thead><tr>`;
          for (let colIdx = 0; colIdx < maxCols; colIdx++) {
            const h = colIdx < headers.length ? headers[colIdx] : "";
            const align = colIdx < alignments.length ? alignments[colIdx] : "left";
            const alignStyle = align !== "left" ? ` style="text-align:${align};"` : "";
            tableHtml += `<th${alignStyle}>${renderInlineMarkdown(h)}</th>`;
          }
          tableHtml += `</tr></thead><tbody>`;
          for (const row of rows) {
            tableHtml += `<tr>`;
            for (let colIdx = 0; colIdx < maxCols; colIdx++) {
              const cellVal = colIdx < row.length ? row[colIdx] : "";
              const align = colIdx < alignments.length ? alignments[colIdx] : "left";
              const alignStyle = align !== "left" ? ` style="text-align:${align};"` : "";
              tableHtml += `<td${alignStyle}>${renderInlineMarkdown(cellVal)}</td>`;
            }
            tableHtml += `</tr>`;
          }
          tableHtml += `</tbody></table></div>`;
          blocks.push(tableHtml);
          continue;
        }
      }
    }

    // 5. Blockquotes & GitHub Alerts
    if (trimmed.startsWith(">")) {
      const quoteLines = [];
      while (i < lines.length) {
        const qLine = lines[i].trim();
        if (qLine.startsWith(">")) {
          quoteLines.push(qLine.replace(/^>\s?/, ""));
          i++;
        } else {
          break;
        }
      }
      const quoteText = quoteLines.join("\n");
      const alertMatch = quoteText.match(/^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]\s*([\s\S]*)$/i);
      if (alertMatch) {
        const alertType = alertMatch[1].toLowerCase();
        const alertContent = alertMatch[2].trim();
        blocks.push(`
          <div class="ios-alert alert-${alertType}">
            <div class="alert-title">${alertMatch[1].toUpperCase()}</div>
            <div class="alert-content">${renderInlineMarkdown(alertContent).replace(/\n/g, "<br/>")}</div>
          </div>
        `);
      } else {
        blocks.push(`<blockquote class="ios-blockquote">${renderInlineMarkdown(quoteText).replace(/\n/g, "<br/>")}</blockquote>`);
      }
      continue;
    }

    // 6. Lists (Unordered & Ordered)
    const isUnordered = trimmed.startsWith("- ") || trimmed.startsWith("* ") || trimmed.startsWith("• ");
    const isOrdered = /^\d+\.\s/.test(trimmed);
    if (isUnordered || isOrdered) {
      const listItems = [];
      const tag = isOrdered ? "ol" : "ul";
      while (i < lines.length) {
        const lLine = lines[i].trim();
        if (isOrdered && /^\d+\.\s/.test(lLine)) {
          listItems.push(lLine.replace(/^\d+\.\s+/, ""));
          i++;
        } else if (!isOrdered && (lLine.startsWith("- ") || lLine.startsWith("* ") || lLine.startsWith("• "))) {
          listItems.push(lLine.replace(/^[-*•]\s+/, ""));
          i++;
        } else {
          break;
        }
      }
      const itemsHtml = listItems.map(it => `<li>${renderInlineMarkdown(it)}</li>`).join("");
      blocks.push(`<${tag} class="ios-list">${itemsHtml}</${tag}>`);
      continue;
    }

    // 7. Paragraph
    const paraLines = [line];
    i++;
    while (i < lines.length) {
      const nextLine = lines[i];
      const nTrimmed = nextLine.trim();
      if (!nTrimmed ||
          nTrimmed.startsWith("```") ||
          nTrimmed.startsWith("#") ||
          nTrimmed === "---" || nTrimmed === "***" || nTrimmed === "___" ||
          (nTrimmed.startsWith("|") && nTrimmed.endsWith("|")) ||
          nTrimmed.startsWith(">") ||
          nTrimmed.startsWith("- ") || nTrimmed.startsWith("* ") || nTrimmed.startsWith("• ") ||
          /^\d+\.\s/.test(nTrimmed)) {
        break;
      }
      paraLines.push(nextLine);
      i++;
    }
    const paraHtml = renderInlineMarkdown(paraLines.join("\n")).replace(/\n/g, "<br/>");
    blocks.push(`<p>${paraHtml}</p>`);
  }

  return blocks.join("");
}

window.copyCode = function(btn) {
  const card = btn.closest(".code-block-card");
  if (!card) return;
  const codeEl = card.querySelector("code");
  if (!codeEl) return;
  const text = codeEl.innerText;
  navigator.clipboard.writeText(text).then(() => {
    const span = btn.querySelector("span");
    if (span) {
      const orig = span.textContent;
      span.textContent = "已复制";
      btn.classList.add("copied");
      setTimeout(() => {
        span.textContent = orig;
        btn.classList.remove("copied");
      }, 1500);
    }
  }).catch(() => {});
};

// Markdown & LaTeX Parsing Memory Cache (LRU)
const markdownCache = new Map();
/** FNV-1a hash for fast full-text cache key generation */
function fnv1aHash(str) {
  let hash = 0x811c9dc5;
  for (let i = 0; i < str.length; i++) {
    hash ^= str.charCodeAt(i);
    hash = (hash * 0x01000193) >>> 0;
  }
  return hash.toString(36);
}
function getCachedMarkdown(md) {
  if (!md) return "";
  const key = fnv1aHash(md);
  if (markdownCache.has(key)) {
    return markdownCache.get(key);
  }
  const html = renderMarkdown(md);
  if (markdownCache.size > 250) {
    const firstKey = markdownCache.keys().next().value;
    markdownCache.delete(firstKey);
  }
  markdownCache.set(key, html);
  return html;
}

// --- Initialization ---

window.addEventListener("DOMContentLoaded", () => {
  if ("serviceWorker" in navigator) {
    navigator.serviceWorker.register("/sw.js").catch(() => {});
  }

  window.addEventListener("hashchange", renderRoute);

  checkGatewayStatus();
  setInterval(checkGatewayStatus, 6000);

  // Navigation & Sheets
  document.getElementById("btn-back")?.addEventListener("click", () => {
    triggerHaptic("selection");
    navigateTo("#");
  });
  document.getElementById("btn-new")?.addEventListener("click", openNewSheet);
  document.getElementById("btn-settings")?.addEventListener("click", openSettingsSheet);

  // New Conversation Sheet
  document.getElementById("btn-sheet-new-cancel")?.addEventListener("click", closeNewSheet);
  document.getElementById("btn-sheet-new-create")?.addEventListener("click", createConversation);
  const sheetNew = document.getElementById("sheet-new");
  sheetNew?.addEventListener("click", (e) => {
    if (e.target === sheetNew) closeNewSheet();
  });

  // Settings Sheet
  document.getElementById("btn-sheet-settings-done")?.addEventListener("click", closeSettingsSheet);
  document.getElementById("btn-rescan-gateway")?.addEventListener("click", rescanGateway);
  document.getElementById("btn-open-pairing")?.addEventListener("click", () => {
    closeSettingsSheet();
    openPairingSheet();
  });
  document.getElementById("btn-unpair-device")?.addEventListener("click", unpairDevice);
  const sheetSettings = document.getElementById("sheet-settings");
  sheetSettings?.addEventListener("click", (e) => {
    if (e.target === sheetSettings) closeSettingsSheet();
  });

  // Pairing Sheet
  document.getElementById("btn-sheet-pairing-cancel")?.addEventListener("click", closePairingSheet);
  document.getElementById("btn-sheet-pairing-submit")?.addEventListener("click", submitPairing);
  const sheetPairing = document.getElementById("sheet-pairing");
  sheetPairing?.addEventListener("click", (e) => {
    if (e.target === sheetPairing) closePairingSheet();
  });

  // iOS Alert Dialog: Rename Conversation
  document.getElementById("btn-alert-rename-cancel")?.addEventListener("click", closeRenameAlert);
  document.getElementById("btn-alert-rename-save")?.addEventListener("click", submitRenameConversation);
  document.getElementById("input-rename-title")?.addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      e.preventDefault();
      submitRenameConversation();
    }
  });
  const alertRename = document.getElementById("alert-rename");
  alertRename?.addEventListener("click", (e) => {
    if (e.target === alertRename) closeRenameAlert();
  });

  // iOS ActionSheet: Delete Conversation Confirmation
  document.getElementById("btn-actionsheet-delete-cancel")?.addEventListener("click", closeDeleteActionSheet);
  document.getElementById("actionsheet-delete-backdrop")?.addEventListener("click", closeDeleteActionSheet);
  document.getElementById("btn-actionsheet-delete-confirm")?.addEventListener("click", confirmDeleteConversation);

  // Initialize iOS Gestures & Viewport Handling
  initPullToRefresh();
  initEdgeSwipeBack();
  initVisualViewportHandling();

  // Check URL parameters for auto pairing
  const urlParams = new URLSearchParams(window.location.search);
  const autoPairCode = urlParams.get("pair_code") || urlParams.get("code");
  if (autoPairCode) {
    pairWithCode(autoPairCode).then(() => {
      window.history.replaceState({}, document.title, window.location.pathname);
      loadConversations();
    }).catch(err => {
      openPairingSheet(err.message);
    });
  }

  updateAuthUI();

  // Action Button (Send / Stop / Queue Toggle)
  const sendBtn = document.getElementById("btn-send");
  sendBtn?.addEventListener("click", () => {
    const summary = currentTrajectories[activeCascadeId];
    const isRunning = summary?.status === "CASCADE_RUN_STATUS_RUNNING";
    const hasContent = (document.getElementById("chat-input")?.value.trim().length > 0) || (pendingImages && pendingImages.length > 0);
    if (isRunning && !hasContent) {
      cancelCurrentTask();
    } else {
      sendMessage();
    }
  });

  // Expand / Collapse Running Tasks Card
  document.getElementById("btn-tasks-expand")?.addEventListener("click", () => {
    RunningTasksManager.toggleExpand();
  });

  // Expand / Collapse Queued Messages Card
  document.getElementById("btn-queued-expand")?.addEventListener("click", () => {
    LocalQueueManager.toggleExpand();
  });

  // Add Image (+) Chip & File Input
  const addImgBtn = document.getElementById("btn-add-image");
  const imgFileInput = document.getElementById("image-file-input");
  addImgBtn?.addEventListener("click", () => {
    imgFileInput?.click();
  });
  imgFileInput?.addEventListener("change", (e) => {
    if (e.target.files && e.target.files.length) {
      handleFilesSelected(e.target.files);
      e.target.value = "";
    }
  });

  // Model Switch Chip
  const modelSwitchBtn = document.getElementById("btn-model-switch");
  modelSwitchBtn?.addEventListener("click", () => {
    toggleModel();
  });
  updateModelSwitchUI();

  // Chips
  document.getElementById("btn-commit-push")?.addEventListener("click", () => {
    const input = document.getElementById("chat-input");
    if (!input) return;
    const toAppend = "Commit and Push";
    if (!input.value.trim()) {
      input.value = toAppend;
    } else {
      input.value += "\n" + toAppend;
    }
    input.focus();
    if (sendBtn && sendBtn.classList.contains("send-mode")) {
      sendBtn.classList.add("active");
    }
  });

  document.getElementById("btn-proceed")?.addEventListener("click", handleProceed);

  // Search Filter with iOS Clear Button
  const searchInput = document.getElementById("conv-search");
  const searchClearBtn = document.getElementById("btn-search-clear");
  if (searchInput) {
    searchInput.addEventListener("input", () => {
      if (searchClearBtn) {
        if (searchInput.value.trim().length > 0) {
          searchClearBtn.classList.remove("hidden");
        } else {
          searchClearBtn.classList.add("hidden");
        }
      }
      renderConversationList(currentTrajectories);
    });
  }
  if (searchClearBtn) {
    searchClearBtn.addEventListener("click", () => {
      if (searchInput) {
        searchInput.value = "";
        searchInput.focus();
      }
      searchClearBtn.classList.add("hidden");
      renderConversationList(currentTrajectories);
    });
  }

  // Chat Input Auto-grow, Keyboard Dismissal Recovery & Dynamic Queue / Stop Controls
  const chatInput = document.getElementById("chat-input");
  const messagesStream = document.getElementById("messages-stream");

  // Tap conversation view to dismiss keyboard smoothly (parity with iOS)
  if (messagesStream) {
    messagesStream.addEventListener("pointerdown", (e) => {
      // Don't blur if tapping inside an input, button, or interactive control
      if (e.target.closest("button, a, input, select, textarea, summary, .chip-pill")) return;
      if (document.activeElement === chatInput) {
        chatInput.blur();
      }
    });
  }

  if (chatInput) {
    chatInput.addEventListener("input", () => {
      chatInput.style.height = "auto";
      chatInput.style.height = Math.min(chatInput.scrollHeight, 120) + "px";
      const isRunning = currentTrajectories[activeCascadeId]?.status === "CASCADE_RUN_STATUS_RUNNING";
      updateChatControls(isRunning, null, false);
    });

    chatInput.addEventListener("focus", () => {
      // Lock window displacement when virtual keyboard rises (parity with iOS)
      window.scrollTo(0, 0);
      document.body.scrollTop = 0;
      document.documentElement.scrollTop = 0;
      setTimeout(() => {
        window.scrollTo(0, 0);
        if (messagesStream && userIsNearBottom) {
          messagesStream.scrollTop = messagesStream.scrollHeight;
        }
      }, 100);
      setTimeout(() => {
        window.scrollTo(0, 0);
        if (messagesStream && userIsNearBottom) {
          messagesStream.scrollTop = messagesStream.scrollHeight;
        }
      }, 320);
    });

    chatInput.addEventListener("blur", () => {
      // Ensure window is not displaced when keyboard retracts without jerking messagesStream
      window.scrollTo(0, 0);
      document.body.scrollTop = 0;
      document.documentElement.scrollTop = 0;
    });

    chatInput.addEventListener("keydown", (e) => {
      if (e.isComposing || e.keyCode === 229) return; // Ignore IME composition (Chinese, Japanese, Korean)
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        sendMessage();
      }
    });

    // Support pasting image screenshots directly into chat input
    chatInput.addEventListener("paste", (e) => {
      const items = e.clipboardData?.items;
      if (!items) return;
      const files = [];
      for (let i = 0; i < items.length; i++) {
        if (items[i].type.startsWith("image/")) {
          const file = items[i].getAsFile();
          if (file) files.push(file);
        }
      }
      if (files.length > 0) {
        handleFilesSelected(files);
      }
    });
  }

  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible" && activeCascadeId) {
      if (!activeWs || activeWs.readyState !== WebSocket.OPEN) {
        connectStreamWs(activeCascadeId);
      }
    }
    if (document.visibilityState === "visible" && typeof fetchCockpitQuotas === "function") {
      fetchCockpitQuotas();
    }
  });

  renderRoute();
  initQuotaModule();
  initMarkdownViewer();
});

// ==========================================================================
// Cockpit Quota Monitor Module
// ==========================================================================
// 功能参数：是否在 Cockpit Tools 页面显示账号切换按钮。
// 当前切换后端逻辑暂不可用，因此默认隐藏（false）；后期调通就绪后，将此参数修改为 true 即可恢复切换按钮。
const ENABLE_COCKPIT_SWITCH = false;

function isCockpitSwitchEnabled() {
  if (typeof window !== "undefined" && typeof window.ENABLE_COCKPIT_SWITCH === "boolean") {
    return window.ENABLE_COCKPIT_SWITCH;
  }
  return ENABLE_COCKPIT_SWITCH;
}

let currentCockpitQuotas = null;

async function fetchCockpitQuotas(isManual = false) {
  const refreshBtn = document.getElementById("btn-quota-refresh");
  const refreshIcon = refreshBtn ? refreshBtn.querySelector(".refresh-icon") : null;
  if (isManual && refreshIcon) {
    refreshIcon.classList.add("spinning");
  }

  try {
    if (isManual) {
      await fetch("/api/v1/cockpit/refresh", { method: "POST" }).catch(() => {});
      await new Promise((r) => setTimeout(r, 600));
    }
    const resp = await fetch("/api/v1/cockpit/quotas");
    if (!resp.ok) throw new Error("HTTP " + resp.status);
    const data = await resp.json();
    currentCockpitQuotas = data;
    renderQuotaStatusBar(data);
    renderQuotaSheet(data);
  } catch (err) {
    console.warn("[CockpitQuota] Failed to fetch quotas:", err);
    const statusDesc = document.getElementById("quota-status-desc");
    if (statusDesc && !currentCockpitQuotas) {
      statusDesc.textContent = "未能连接 Cockpit";
    }
  } finally {
    if (refreshIcon) {
      refreshIcon.classList.remove("spinning");
    }
  }
}

function getQuotaStatusClass(percent) {
  if (percent > 50) return "good";
  if (percent >= 20) return "warning";
  return "danger";
}

function formatResetClockTime(isoStr) {
  if (!isoStr) return "";
  try {
    const d = new Date(isoStr);
    if (isNaN(d.getTime())) return "";
    const hours = String(d.getHours()).padStart(2, '0');
    const minutes = String(d.getMinutes()).padStart(2, '0');
    return `(${hours}:${minutes})`;
  } catch (e) {
    return "";
  }
}

function formatResetDateTime(isoStr) {
  if (!isoStr) return "";
  try {
    const d = new Date(isoStr);
    if (isNaN(d.getTime())) return "";
    const month = String(d.getMonth() + 1).padStart(2, '0');
    const day = String(d.getDate()).padStart(2, '0');
    const hours = String(d.getHours()).padStart(2, '0');
    const minutes = String(d.getMinutes()).padStart(2, '0');
    return `(${month}/${day} ${hours}:${minutes})`;
  } catch (e) {
    return "";
  }
}

function getQuotaResetDisplayText(bucket) {
  if (!bucket || !bucket.reset_friendly) return "配额充足";
  const friendly = bucket.reset_friendly;
  if (friendly === "已就绪" || friendly === "未知" || friendly === "配额充足" || friendly === "就绪") {
    return friendly;
  }
  const dateTime = formatResetDateTime(bucket.reset_time);
  if (dateTime) {
    return `${friendly} ${dateTime}`;
  }
  return friendly;
}

function renderQuotaStatusBar(data) {
  if (!data) return;
  const current = data.current_account || (data.accounts && data.accounts[0]);
  if (!current || !current.gemini_5h) return;

  const percentEl = document.getElementById("quota-status-percent");
  const fillEl = document.getElementById("quota-status-fill");
  const descEl = document.getElementById("quota-status-desc");

  const pct = current.gemini_5h.remaining_percent;
  const statusClass = getQuotaStatusClass(pct);

  if (percentEl) percentEl.textContent = `${pct.toFixed(1)}%`;
  if (fillEl) {
    fillEl.style.width = `${Math.min(100, Math.max(0, pct))}%`;
    fillEl.className = `quota-progress-fill fill-${statusClass}`;
  }
  if (descEl) {
    const resetTxt = current.gemini_5h.reset_friendly || "就绪";
    let displayText = resetTxt;
    if (resetTxt !== "就绪" && resetTxt !== "已就绪" && resetTxt !== "未知") {
      const dateTime = formatResetDateTime(current.gemini_5h.reset_time);
      if (dateTime) {
        displayText = `${resetTxt} ${dateTime}`;
      }
    }
    descEl.textContent = displayText;
  }
}

let isCockpitEmailMasked = localStorage.getItem("cockpit_email_masked") === "true";

function maskEmail(email) {
  if (!email || typeof email !== "string") return email;
  const atIdx = email.indexOf("@");
  if (atIdx <= 0) return email;
  const local = email.slice(0, atIdx);
  const domain = email.slice(atIdx + 1);

  const maskPart = (str) => {
    if (str.length <= 1) return str + "*";
    if (str.length === 2) return str[0] + "*" + str[1];
    return str[0] + "*".repeat(str.length - 2) + str[str.length - 1];
  };

  const dotIdx = domain.lastIndexOf(".");
  if (dotIdx > 0) {
    const domainName = domain.slice(0, dotIdx);
    const domainExt = domain.slice(dotIdx);
    return `${maskPart(local)}@${maskPart(domainName)}${domainExt}`;
  }
  return `${maskPart(local)}@${maskPart(domain)}`;
}

function buildAccountQuotaCard(acc, isCurrent) {
  const g5h = acc.gemini_5h || { remaining_percent: 0, reset_friendly: "未知" };
  const gWk = acc.gemini_weekly || { remaining_percent: 0, reset_friendly: "未知" };
  const c5h = acc.claude_5h || { remaining_percent: 0, reset_friendly: "未知" };
  const cWk = acc.claude_weekly || { remaining_percent: 0, reset_friendly: "未知" };

  const card = document.createElement("div");
  card.className = `quota-account-card ${isCurrent ? "current-account-card" : ""}`;

  const displayEmail = isCockpitEmailMasked ? maskEmail(acc.email) : acc.email;
  const switchBtnHtml = (!isCurrent && isCockpitSwitchEnabled())
    ? `<button class="quota-switch-btn" data-id="${escapeHtml(acc.id)}" data-email="${escapeHtml(acc.email)}">切换</button>`
    : "";

  card.innerHTML = `
    <div class="quota-card-header">
      <div class="quota-card-identity">
        <span class="quota-account-email" title="${escapeHtml(acc.email)}">${escapeHtml(displayEmail)}</span>
      </div>
      ${isCurrent ? `<span class="quota-active-tag">🟢 使用中</span>` : switchBtnHtml}
    </div>

    <div class="quota-metrics-grid">
      <!-- 1. Left Top: Claude 5h -->
      <div class="metric-box">
        <div class="metric-box-header">
          <span class="metric-box-title claude">🟣 Claude 5h</span>
          <span class="metric-box-value ${getQuotaStatusClass(c5h.remaining_percent)}">${c5h.remaining_percent.toFixed(1)}%</span>
        </div>
        <div class="metric-mini-track">
          <div class="metric-mini-fill ${getQuotaStatusClass(c5h.remaining_percent)}" style="width: ${Math.min(100, Math.max(0, c5h.remaining_percent))}%;"></div>
        </div>
        <span class="metric-box-reset" title="${escapeHtml(getQuotaResetDisplayText(c5h))}">${escapeHtml(getQuotaResetDisplayText(c5h))}</span>
      </div>

      <!-- 2. Right Top: Gemini 5h -->
      <div class="metric-box">
        <div class="metric-box-header">
          <span class="metric-box-title gemini">🔵 Gemini 5h</span>
          <span class="metric-box-value ${getQuotaStatusClass(g5h.remaining_percent)}">${g5h.remaining_percent.toFixed(1)}%</span>
        </div>
        <div class="metric-mini-track">
          <div class="metric-mini-fill ${getQuotaStatusClass(g5h.remaining_percent)}" style="width: ${Math.min(100, Math.max(0, g5h.remaining_percent))}%;"></div>
        </div>
        <span class="metric-box-reset" title="${escapeHtml(getQuotaResetDisplayText(g5h))}">${escapeHtml(getQuotaResetDisplayText(g5h))}</span>
      </div>

      <!-- 3. Left Bottom: Claude Weekly -->
      <div class="metric-box">
        <div class="metric-box-header">
          <span class="metric-box-title claude">🟣 Claude Weekly</span>
          <span class="metric-box-value ${getQuotaStatusClass(cWk.remaining_percent)}">${cWk.remaining_percent.toFixed(1)}%</span>
        </div>
        <div class="metric-mini-track">
          <div class="metric-mini-fill ${getQuotaStatusClass(cWk.remaining_percent)}" style="width: ${Math.min(100, Math.max(0, cWk.remaining_percent))}%;"></div>
        </div>
        <span class="metric-box-reset" title="${escapeHtml(getQuotaResetDisplayText(cWk))}">${escapeHtml(getQuotaResetDisplayText(cWk))}</span>
      </div>

      <!-- 4. Right Bottom: Gemini Weekly -->
      <div class="metric-box">
        <div class="metric-box-header">
          <span class="metric-box-title gemini">🔵 Gemini Weekly</span>
          <span class="metric-box-value ${getQuotaStatusClass(gWk.remaining_percent)}">${gWk.remaining_percent.toFixed(1)}%</span>
        </div>
        <div class="metric-mini-track">
          <div class="metric-mini-fill ${getQuotaStatusClass(gWk.remaining_percent)}" style="width: ${Math.min(100, Math.max(0, gWk.remaining_percent))}%;"></div>
        </div>
        <span class="metric-box-reset" title="${escapeHtml(getQuotaResetDisplayText(gWk))}">${escapeHtml(getQuotaResetDisplayText(gWk))}</span>
      </div>
    </div>
  `;

  if (!isCurrent && isCockpitSwitchEnabled()) {
    const switchBtn = card.querySelector(".quota-switch-btn");
    if (switchBtn) {
      switchBtn.addEventListener("click", (e) => {
        e.stopPropagation();
        switchCockpitAccount(acc.id, acc.email, switchBtn);
      });
    }
  }

  return card;
}

async function switchCockpitAccount(accountId, accountEmail, btn) {
  if (!accountId) return;
  const originalText = btn ? btn.textContent : "切换";
  if (btn) {
    btn.disabled = true;
    btn.textContent = "切换中...";
    btn.classList.add("switching");
  }

  const lastUpEl = document.getElementById("quota-last-updated");
  const prevSubtitle = lastUpEl ? lastUpEl.textContent : "";
  if (lastUpEl) {
    lastUpEl.textContent = "正在切换账号...";
  }

  try {
    const resp = await fetch("/api/v1/cockpit/switch", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ account_id: accountId }),
    });
    const data = await resp.json();
    if (!resp.ok || data.error) {
      throw new Error(data.error || ("HTTP " + resp.status));
    }
    if (lastUpEl) {
      lastUpEl.textContent = "切换成功，正在刷新...";
    }
    await new Promise((r) => setTimeout(r, 600));
    await fetchCockpitQuotas(false);
  } catch (err) {
    console.error("[Cockpit] Switch failed:", err);
    alert("切换账号失败: " + err.message);
    if (lastUpEl) {
      lastUpEl.textContent = prevSubtitle;
    }
    if (btn) {
      btn.disabled = false;
      btn.textContent = originalText;
      btn.classList.remove("switching");
    }
  }
}

function renderQuotaSheet(data) {
  if (!data) return;

  const lastUpEl = document.getElementById("quota-last-updated");
  if (lastUpEl && data.updated_at) {
    const dt = new Date(data.updated_at);
    lastUpEl.textContent = `更新于 ${dt.toLocaleTimeString()}`;
  }

  const currentContainer = document.getElementById("quota-current-card");
  if (currentContainer) {
    currentContainer.innerHTML = "";
    if (data.current_account) {
      currentContainer.appendChild(buildAccountQuotaCard(data.current_account, true));
    }
  }

  const otherContainer = document.getElementById("quota-other-list");
  if (otherContainer) {
    otherContainer.innerHTML = "";
    const otherAccounts = (data.accounts || []).filter(
      (a) => !data.current_account || a.id !== data.current_account.id
    );
    if (otherAccounts.length === 0) {
      otherContainer.innerHTML = `<div style="text-align:center;color:var(--ios-tertiary-label);padding:16px;">无其他备用账号</div>`;
    } else {
      otherAccounts.forEach((acc) => {
        otherContainer.appendChild(buildAccountQuotaCard(acc, false));
      });
    }
  }
}

function openQuotaSheet() {
  const sheet = document.getElementById("sheet-quota");
  if (sheet) {
    sheet.classList.remove("hidden");
    if (currentCockpitQuotas) {
      renderQuotaSheet(currentCockpitQuotas);
    } else {
      fetchCockpitQuotas();
    }
  }
}

function closeQuotaSheet() {
  const sheet = document.getElementById("sheet-quota");
  if (sheet) {
    sheet.classList.add("hidden");
  }
}

function initQuotaModule() {
  const statusBar = document.getElementById("quota-status-bar");
  if (statusBar) {
    statusBar.addEventListener("click", openQuotaSheet);
  }

  const closeBtn = document.getElementById("btn-quota-close");
  if (closeBtn) {
    closeBtn.addEventListener("click", closeQuotaSheet);
  }

  const sheet = document.getElementById("sheet-quota");
  if (sheet) {
    sheet.addEventListener("click", (e) => {
      if (e.target === sheet) {
        closeQuotaSheet();
      }
    });
  }

  const maskBtn = document.getElementById("btn-quota-mask");
  if (maskBtn) {
    if (isCockpitEmailMasked) {
      maskBtn.classList.add("active");
    }
    maskBtn.addEventListener("click", () => {
      isCockpitEmailMasked = !isCockpitEmailMasked;
      localStorage.setItem("cockpit_email_masked", isCockpitEmailMasked ? "true" : "false");
      maskBtn.classList.toggle("active", isCockpitEmailMasked);
      if (currentCockpitQuotas) {
        renderQuotaSheet(currentCockpitQuotas);
      }
    });
  }

  // Initial fetch
  fetchCockpitQuotas();

  // Periodic poll every 5 minutes (300,000 ms) while app is open
  setInterval(() => {
    if (document.visibilityState === "visible") {
      fetchCockpitQuotas();
    }
  }, 5 * 60 * 1000);
}