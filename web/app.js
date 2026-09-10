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

// --- ConnectRPC & Gateway API ---

async function rpc(method, body = {}) {
  const resp = await fetch(`/api/exa.language_server_pb.LanguageServerService/${method}`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "Connect-Protocol-Version": "1"
    },
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

    // View toggling
    convView.classList.remove("active");
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

    chatView.classList.remove("active");
    convView.classList.add("active");

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
      const hasAction = !!item.needsInput;
      const isRunning = item.status === "CASCADE_RUN_STATUS_RUNNING" && !hasAction;
      const isUnread = !isRunning && !hasAction && isConversationUnread(item);
      const unreadDotHtml = isUnread
        ? `<div class="status-unread-dot" title="未读新消息" data-testid="status-unread-dot"><div class="dot-halo"></div><div class="dot-core"></div></div>`
        : "";
      const badgeHtml = hasAction
        ? `<span class="badge badge-action">ACTION</span>`
        : (isRunning ? `<span class="badge badge-running">RUNNING</span>` : unreadDotHtml);
      const title = item.annotations?.title || item.summary || "未命名会话";
      const wsUri = item.workspaceUris?.[0] || item.workspaces?.[0]?.workspaceFolderAbsoluteUri || "";
      const wsName = wsUri.split("/").filter(Boolean).pop() || "workspace";
      const timeStr = formatRelativeTime(item.lastModifiedTime);

      return `
        <div class="conv-card" onclick="navigateTo('#c=${item.id}')">
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
      `;
    })
    .join("");
}

// --- Chat View & Real-Time Stream ---

let activeWs = null;
let wsReconnectTimer = null;
let userIsNearBottom = true;
let hasInitiallyAligned = false;
let prevWasRunning = false;
let currentCanProceed = false;
let currentProceedArtifactUri = null;

function updateProceedButton(canProceed) {
  const proceedBtn = document.getElementById("btn-proceed");
  if (!proceedBtn) return;
  if (canProceed) {
    proceedBtn.classList.remove("hidden");
  } else {
    proceedBtn.classList.add("hidden");
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
  streamEl.addEventListener("scroll", () => {
    const dist = streamEl.scrollHeight - streamEl.scrollTop - streamEl.clientHeight;
    userIsNearBottom = dist <= 90;
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
    const hasText = chatInput && chatInput.value.trim().length > 0;

    if (isRunning) {
      if (hasText) {
        sendBtn.className = "btn-action-circle send-mode active";
        sendBtn.title = "加入待发送队列 (Queue Message)";
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
      if (hasText) sendBtn.classList.add("active");
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

        if (data.steps) {
          renderMessages(data.steps, isRunning);
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
    renderMessages(steps, isRunning);

    fetch(`/gateway/cascade/messages?cascadeId=${encodeURIComponent(cascadeId)}&limit=1`)
      .then(res => res.json())
      .then(info => {
        if (activeCascadeId === cascadeId) {
          if (info.queuedMessages) {
            LocalQueueManager.syncFromServer(info.queuedMessages);
          }
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
      const userText = s.userInput?.userResponse || s.userInput?.items?.[0]?.text || "";
      items.push({
        type: "user",
        id: `item-user-${i}`,
        index: i,
        text: userText,
        step: s
      });
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
      } else {
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
          name: "planning",
          detail: "",
          status: "DONE",
          raw: s
        });
      }
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
    return `u:${item.text.length}:${item.text.slice(-10)}`;
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
  return "";
}

function generateItemHtml(item, isRunning, isLastItem) {
  if (item.type === "user") {
    return `<div class="bubble">${escapeHtml(item.text)}</div>`;
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
      newEl.className = rowClass;
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

  // Standalone Agent Thinking Indicator (shown only while awaiting response right after user input)
  const lastItem = items[items.length - 1];
  const isAwaiting = isRunning && lastItem?.type === "user";
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

  // 1. First-time render on entering a conversation: align INSTANTLY with zero jitter
  if (!hasInitiallyAligned) {
    hasInitiallyAligned = true;
    prevWasRunning = isRunning;

    if (!isRunning && lastItem && lastItem.type === "agent") {
      let lastUserIdx = -1;
      for (let i = items.length - 1; i >= 0; i--) {
        if (items[i].type === "user") {
          lastUserIdx = i;
          break;
        }
      }
      const turnStartIdx = lastUserIdx !== -1 ? lastUserIdx + 1 : 0;
      const turnStartEl = document.getElementById(items[turnStartIdx]?.id);
      if (turnStartEl) {
        turnStartEl.scrollIntoView({ behavior: "instant", block: "start" });
        return;
      }
    }
    streamEl.scrollTop = streamEl.scrollHeight;
    return;
  }

  // 2. Active task finished: smooth scroll to turn start ONCE
  const justFinished = (prevWasRunning && !isRunning);
  prevWasRunning = isRunning;

  if (justFinished) {
    LocalQueueManager.onAgentCompleted();
    if (lastItem && lastItem.type === "agent") {
      let lastUserIdx = -1;
      for (let i = items.length - 1; i >= 0; i--) {
        if (items[i].type === "user") {
          lastUserIdx = i;
          break;
        }
      }
      const turnStartIdx = lastUserIdx !== -1 ? lastUserIdx + 1 : 0;
      const turnStartEl = document.getElementById(items[turnStartIdx]?.id);
      if (turnStartEl && userIsNearBottom) {
        turnStartEl.scrollIntoView({ behavior: "smooth", block: "start" });
        return;
      }
    }
  }

  // 3. Live streaming while running: pin to bottom
  if (isRunning && hasDOMChanges && userIsNearBottom) {
    streamEl.scrollTop = streamEl.scrollHeight;
  }
}

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
          <button class="queued-icon-btn btn-send-now" onclick="LocalQueueManager.sendNow('${item.id}')" title="Send Now" aria-label="Send Now">
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 -960 960 960" fill="currentColor">
              <path d="M665.08-450H180v-60H665.08L437.23-737.85L480-780L780-480L480-180l-42.77-42.15L665.08-450Z"></path>
            </svg>
          </button>
          <button class="queued-icon-btn btn-edit" onclick="LocalQueueManager.edit('${item.id}')" title="Edit" aria-label="Edit">
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 -960 960 960" fill="currentColor">
              <path d="M200-200h50.46L659.92-609.46l-50.46-50.46L200-250.46V-200Zm-60,60V-275.38L667.62-802.77q9.07-8.24 20.04-12.74T710.65-820t23.31,4.27t19.97,13.58l48.85,49.46q9.31,8.69 13.27,20T820-710.07q0,12.07-4.12,23.03T802.77-667L275.38-140H140ZM760.38-710.15l-50.23-50.23l50.23,50.23Zm-126.13,75.9l-24.79-25.67l50.46,50.46l-25.67-24.79Z"></path>
            </svg>
          </button>
          <button class="queued-icon-btn btn-delete" onclick="LocalQueueManager.remove('${item.id}')" title="Delete" aria-label="Delete">
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 -960 960 960" fill="currentColor">
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
  if (!text) return;

  isSendingMessage = true;
  const isRunning = currentTrajectories[activeCascadeId]?.status === "CASCADE_RUN_STATUS_RUNNING";

  inputEl.value = "";
  inputEl.style.height = "auto";
  currentCanProceed = false;
  updateProceedButton(false);

  if (isRunning) {
    // Enqueue message while agent is running
    LocalQueueManager.enqueue(text);
    updateChatControls(true, null, false);
    try {
      await rpc("SendUserCascadeMessage", {
        cascadeId: activeCascadeId,
        items: [{ text }],
        deliveryStrategy: 2 // WHEN_IDLE
      });
    } catch (err) {
      console.warn("[Queue] SendUserCascadeMessage with WHEN_IDLE notification:", err);
    } finally {
      isSendingMessage = false;
    }
    return;
  }

  const streamEl = document.getElementById("messages-stream");
  const tempId = `temp-user-${Date.now()}`;
  streamEl.insertAdjacentHTML("beforeend", `
    <div id="${tempId}" class="message-row user">
      <div class="bubble">${escapeHtml(text)}</div>
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

    await rpc("SendUserCascadeMessage", {
      cascadeId: activeCascadeId,
      items: [{ text }]
    });

    // Ensure WebSocket stream is actively connected
    if (!activeWs || activeWs.readyState !== WebSocket.OPEN) {
      connectStreamWs(activeCascadeId);
    }
  } catch (err) {
    alert("发送失败: " + err.message);
    const tempEl = document.getElementById(tempId);
    if (tempEl) tempEl.remove();
    inputEl.value = text;
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
  const tempId = `temp-user-${Date.now()}`;
  streamEl.insertAdjacentHTML("beforeend", `
    <div id="${tempId}" class="message-row user">
      <div class="bubble">Proceed</div>
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

    await rpc("SendUserCascadeMessage", {
      cascadeId: activeCascadeId,
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
    const tempEl = document.getElementById(tempId);
    if (tempEl) tempEl.remove();
    updateProceedButton(true);
    currentCanProceed = true;
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
  const model = document.getElementById("new-model").value;
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

const latexMathMap = [
  [/\\mathbb\{R\}/g, "ℝ"], [/\\mathbf\{R\}/g, "ℝ"],
  [/\\mathbb\{N\}/g, "ℕ"], [/\\mathbf\{N\}/g, "ℕ"],
  [/\\mathbb\{Z\}/g, "ℤ"], [/\\mathbf\{Z\}/g, "ℤ"],
  [/\\mathbb\{Q\}/g, "ℚ"], [/\\mathbf\{Q\}/g, "ℚ"],
  [/\\mathbb\{C\}/g, "ℂ"], [/\\mathbf\{C\}/g, "ℂ"],
  [/\\mathbb\{E\}/g, "𝔼"], [/\\mathbb\{P\}/g, "ℙ"],
  [/\\mathcal\{L\}/g, "ℒ"], [/\\mathcal\{O\}/g, "𝒪"],
  [/\\longrightarrow/g, "⟶"], [/\\longleftarrow/g, "⟵"],
  [/\\longleftrightarrow/g, "⟷"], [/\\Longrightarrow/g, "⟹"],
  [/\\Longleftarrow/g, "⟸"], [/\\Longleftrightarrow/g, "⟺"],
  [/\\rightleftharpoons/g, "⇌"], [/\\hookrightarrow/g, "↪"], [/\\hookleftarrow/g, "↩"],
  [/\\rightarrow/g, "→"], [/\\leftarrow/g, "←"],
  [/\\leftrightarrow/g, "↔"], [/\\Rightarrow/g, "⇒"], [/\\Leftarrow/g, "⇐"], [/\\Leftrightarrow/g, "⇔"],
  [/\\to\b/g, "→"], [/\\gets\b/g, "←"], [/\\implies/g, "⇒"], [/\\iff/g, "⇔"],
  [/\\uparrow/g, "↑"], [/\\downarrow/g, "↓"], [/\\updownarrow/g, "↕"],
  [/\\Uparrow/g, "⇑"], [/\\Downarrow/g, "⇓"], [/\\Updownarrow/g, "⇕"],
  [/\\nearrow/g, "↗"], [/\\searrow/g, "↘"], [/\\swarrow/g, "↙"], [/\\nwarrow/g, "↖"],
  [/\\mapsto/g, "↦"], [/\\longmapsto/g, "⟼"],
  [/\\leqslant/g, "≤"], [/\\geqslant/g, "≥"], [/\\leq/g, "≤"], [/\\geq/g, "≥"],
  [/\\le\b/g, "≤"], [/\\ge\b/g, "≥"], [/\\neq/g, "≠"], [/\\ne\b/g, "≠"],
  [/\\approx/g, "≈"], [/\\simeq/g, "≃"], [/\\cong/g, "≅"], [/\\equiv/g, "≡"],
  [/\\propto/g, "∝"], [/\\ll/g, "≪"], [/\\gg/g, "≫"], [/\\parallel/g, "∥"], [/\\perp/g, "⊥"],
  [/\\sim/g, "∼"], [/\\subset/g, "⊂"], [/\\supset/g, "⊃"],
  [/\\subseteq/g, "⊆"], [/\\supseteq/g, "⊇"], [/\\subsetneq/g, "⊊"], [/\\supsetneq/g, "⊋"],
  [/\\notin/g, "∉"], [/\\in\b/g, "∈"], [/\\cup/g, "∪"], [/\\cap/g, "∩"], [/\\setminus/g, "∖"],
  [/\\emptyset/g, "∅"], [/\\empty\b/g, "∅"], [/\\forall/g, "∀"], [/\\exists/g, "∃"],
  [/\\times/g, "×"], [/\\div/g, "÷"], [/\\pm/g, "±"], [/\\mp/g, "∓"],
  [/\\cdot/g, "·"], [/\\cdots/g, "⋯"], [/\\ldots/g, "…"], [/\\vdots/g, "⋮"], [/\\ddots/g, "⋱"],
  [/\\bullet/g, "•"], [/\\circ/g, "∘"], [/\\star/g, "⋆"], [/\\ast/g, "∗"],
  [/\\oplus/g, "⊕"], [/\\ominus/g, "⊖"], [/\\otimes/g, "⊗"], [/\\odot/g, "⊙"],
  [/\\iiint/g, "∭"], [/\\iint/g, "∬"], [/\\oint/g, "∮"], [/\\int/g, "∫"],
  [/\\sum/g, "∑"], [/\\prod/g, "∏"], [/\\partial/g, "∂"], [/\\nabla/g, "∇"], [/\\infty/g, "∞"], [/\\sqrt/g, "√"],
  [/\\degree/g, "°"],
  [/\\Gamma/g, "Γ"], [/\\Delta/g, "Δ"], [/\\Theta/g, "Θ"], [/\\Lambda/g, "Λ"], [/\\Xi/g, "Ξ"],
  [/\\Pi/g, "Π"], [/\\Sigma/g, "Σ"], [/\\Upsilon/g, "Υ"], [/\\Phi/g, "Φ"], [/\\Psi/g, "Ψ"], [/\\Omega/g, "Ω"],
  [/\\alpha/g, "α"], [/\\beta/g, "β"], [/\\gamma/g, "γ"], [/\\delta/g, "δ"],
  [/\\varepsilon/g, "ε"], [/\\epsilon/g, "ϵ"], [/\\zeta/g, "ζ"], [/\\eta/g, "η"],
  [/\\vartheta/g, "ϑ"], [/\\theta/g, "θ"], [/\\iota/g, "ι"], [/\\kappa/g, "κ"],
  [/\\lambda/g, "λ"], [/\\mu/g, "μ"], [/\\nu/g, "ν"], [/\\xi/g, "ξ"], [/\\pi/g, "π"],
  [/\\varrho/g, "ϱ"], [/\\rho/g, "ρ"], [/\\varsigma/g, "ς"], [/\\sigma/g, "σ"],
  [/\\tau/g, "τ"], [/\\upsilon/g, "υ"], [/\\varphi/g, "φ"], [/\\phi/g, "ϕ"],
  [/\\chi/g, "χ"], [/\\psi/g, "ψ"], [/\\omega/g, "ω"]
];

const supMap = { "0": "⁰", "1": "¹", "2": "²", "3": "³", "4": "⁴", "5": "⁵", "6": "⁶", "7": "⁷", "8": "⁸", "9": "⁹", "+": "⁺", "-": "⁻", "=": "⁼", "(": "⁽", ")": "⁾", "n": "ⁿ", "i": "ⁱ", "j": "ʲ", "a": "ᵃ", "b": "ᵇ", "c": "ᶜ", "d": "ᵈ", "e": "ᵉ", "f": "ᶠ", "g": "ᵍ", "h": "ʰ", "k": "ᵏ", "l": "ˡ", "m": "ᵐ", "o": "ᵒ", "p": "ᵖ", "r": "ʳ", "s": "ˢ", "t": "ᵗ", "u": "ᵘ", "v": "ᵛ", "w": "ʷ", "x": "ˣ", "y": "ʸ", "z": "ᶻ", "T": "ᵀ" };
const subMap = { "0": "₀", "1": "₁", "2": "₂", "3": "₃", "4": "₄", "5": "₅", "6": "₆", "7": "₇", "8": "₈", "9": "₉", "+": "₊", "-": "₋", "=": "₌", "(": "₍", ")": "₎", "a": "ₐ", "e": "ₑ", "h": "ₕ", "i": "ᵢ", "j": "ⱼ", "k": "ₖ", "l": "ₗ", "m": "ₘ", "n": "ₙ", "o": "ₒ", "p": "ₚ", "r": "ᵣ", "s": "ₛ", "t": "ₜ", "u": "ᵤ", "v": "ᵥ", "x": "ₓ" };

function cleanMathExpr(str) {
  str = str.replace(/\\(?:text|mathrm|mathbf|mathit|operatorname)\{([^}]*)\}/g, "$1");
  str = str.replace(/\\frac\{([^}]*)\}\{([^}]*)\}/g, "$1 / $2");
  str = str.replace(/\\sqrt\{([^}]*)\}/g, "√($1)");
  str = str.replace(/\\left\(/g, "(").replace(/\\right\)/g, ")");
  str = str.replace(/\\left\[/g, "[").replace(/\\right\]/g, "]");
  str = str.replace(/\\left\\\{/g, "{").replace(/\\right\\\}/g, "}");
  str = str.replace(/\\\{/g, "{").replace(/\\\}/g, "}");
  str = str.replace(/\\%/g, "%").replace(/\\_/g, "_").replace(/\\&/g, "&");
  str = str.replace(/\\,/g, " ").replace(/\\;/g, " ").replace(/\\quad/g, " ").replace(/\\qquad/g, "  ");
  for (const [re, repl] of latexMathMap) {
    str = str.replace(re, repl);
  }
  str = str.replace(/\^\{([0-9a-zA-Z\+\-\=\(\)]+)\}/g, (_, chars) => chars.split("").map(c => supMap[c] || c).join(""));
  str = str.replace(/\^([0-9a-zA-Z\+\-\*])/g, (_, c) => supMap[c] || c);
  str = str.replace(/_\{([0-9a-zA-Z\+\-\=\(\)]+)\}/g, (_, chars) => chars.split("").map(c => subMap[c] || c).join(""));
  str = str.replace(/_([0-9a-zA-Z])/g, (_, c) => subMap[c] || c);
  return str.trim();
}

function processMathSymbols(text) {
  if (!text) return "";
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

  for (const [re, repl] of latexMathMap) {
    text = text.replace(re, repl);
  }

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
    if (icon) {
      return `<a href="${url}" class="file-link" target="_blank" rel="noopener noreferrer"><img src="/icons/files/${icon}.svg" class="file-icon" alt="" /><span>${linkText}</span></a>`;
    }
    return `<a href="${url}" class="text-link" target="_blank" rel="noopener noreferrer">${linkText}</a>`;
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
          const parts = rowStr.split("|");
          if (parts.length < 2) return [];
          return parts.slice(1, parts.length - 1).map(c => c.trim());
        };
        const headers = parseTableRow(tableLines[0]);
        const rows = [];
        for (let rIdx = 1; rIdx < tableLines.length; rIdx++) {
          const r = parseTableRow(tableLines[rIdx]);
          // Skip separator row (| --- | :--- |)
          const isSep = r.every(cell => /^[\s\-:]+$/.test(cell));
          if (isSep) continue;
          rows.push(r);
        }

        let tableHtml = `<div class="table-wrapper"><table class="ios-markdown-table"><thead><tr>`;
        for (const h of headers) {
          tableHtml += `<th>${renderInlineMarkdown(h)}</th>`;
        }
        tableHtml += `</tr></thead><tbody>`;
        for (const row of rows) {
          tableHtml += `<tr>`;
          for (let colIdx = 0; colIdx < headers.length; colIdx++) {
            const cellVal = colIdx < row.length ? row[colIdx] : "";
            tableHtml += `<td>${renderInlineMarkdown(cellVal)}</td>`;
          }
          tableHtml += `</tr>`;
        }
        tableHtml += `</tbody></table></div>`;
        blocks.push(tableHtml);
        continue;
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
  document.getElementById("btn-back")?.addEventListener("click", () => navigateTo("#"));
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
    const hasText = document.getElementById("chat-input")?.value.trim().length > 0;
    if (isRunning && !hasText) {
      cancelCurrentTask();
    } else {
      sendMessage();
    }
  });

  // Expand / Collapse Queued Messages Card
  document.getElementById("btn-queued-expand")?.addEventListener("click", () => {
    LocalQueueManager.toggleExpand();
  });

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

  // Chat Input Auto-grow & Dynamic Queue / Stop Controls
  const chatInput = document.getElementById("chat-input");
  if (chatInput) {
    chatInput.addEventListener("input", () => {
      chatInput.style.height = "auto";
      chatInput.style.height = Math.min(chatInput.scrollHeight, 120) + "px";
      const isRunning = currentTrajectories[activeCascadeId]?.status === "CASCADE_RUN_STATUS_RUNNING";
      updateChatControls(isRunning, null, false);
    });
    chatInput.addEventListener("keydown", (e) => {
      if (e.isComposing || e.keyCode === 229) return; // Ignore IME composition (Chinese, Japanese, Korean)
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        sendMessage();
      }
    });
  }

  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible" && activeCascadeId) {
      if (!activeWs || activeWs.readyState !== WebSocket.OPEN) {
        connectStreamWs(activeCascadeId);
      }
    }
  });

  renderRoute();
});