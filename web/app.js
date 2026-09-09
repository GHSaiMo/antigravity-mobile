// Antigravity Mobile Gateway - Web & PWA Client

let activeCascadeId = null;
let pollTimer = null;
let currentTrajectories = {};
let availableModels = [];

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
  const pill = document.getElementById("conn-pill");
  if (!pill) return;
  const text = pill.querySelector(".status-text");

  try {
    const resp = await fetch("/gateway/status");
    const data = await resp.json();

    if (data.status === "connected" && data.upstream) {
      pill.className = "status-pill connected";
      text.textContent = `已连接 :${data.upstream.port}`;
      pill.title = `PID: ${data.upstream.pid} | Token: ${data.upstream.csrf_token.slice(0, 8)}... (点击重新探测)`;
    } else {
      pill.className = "status-pill disconnected";
      text.textContent = "未连接";
      pill.title = "language_server 未启动 (点击重试)";
    }
  } catch (e) {
    pill.className = "status-pill disconnected";
    text.textContent = "网关离线";
  }
}

async function rescanGateway() {
  const pill = document.getElementById("conn-pill");
  pill.className = "status-pill discovering";
  pill.querySelector(".status-text").textContent = "正在探测...";

  try {
    const resp = await fetch("/gateway/rescan");
    await resp.json();
    await checkGatewayStatus();
    loadConversations();
  } catch (e) {
    await checkGatewayStatus();
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
  const backBtn = document.getElementById("btn-back");
  const title = document.getElementById("header-title");
  const subtitle = document.getElementById("header-subtitle");

  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }

  if (hash.startsWith("#c=")) {
    const newCascadeId = hash.slice(3);
    const changed = activeCascadeId !== newCascadeId;
    activeCascadeId = newCascadeId;

    convView.classList.remove("active");
    chatView.classList.add("active");
    backBtn.classList.remove("hidden");

    title.textContent = "会话详情";
    if (subtitle) subtitle.textContent = activeCascadeId.slice(0, 8);

    if (changed) {
      hasInitiallyAligned = false;
      prevWasRunning = false;
      const streamEl = document.getElementById("messages-stream");
      if (streamEl) {
        streamEl.innerHTML = `
          <div class="loading-state">
            <div class="spinner"></div>
            <p>正在同步会话历史与步骤...</p>
          </div>
        `;
      }
    }

    // Connect real-time WebSocket stream (WS delivers snapshot directly)
    connectStreamWs(activeCascadeId);
  } else {
    activeCascadeId = null;
    closeActiveWs();

    chatView.classList.remove("active");
    convView.classList.add("active");
    backBtn.classList.add("hidden");

    title.textContent = "Antigravity";
    if (subtitle) subtitle.textContent = "";

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
        <p style="color: var(--status-error);">加载失败: ${escapeHtml(err.message)}</p>
        <button class="btn-secondary" onclick="loadConversations()" style="margin-top:10px;">重试</button>
      </div>
    `;
  }
}

function renderConversationList(summaries) {
  const listEl = document.getElementById("conversations-list");
  const query = (document.getElementById("conv-search").value || "").toLowerCase().trim();

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
      const isRunning = item.status === "CASCADE_RUN_STATUS_RUNNING";
      const badgeClass = isRunning ? "badge-running" : "badge-idle";
      const badgeText = isRunning ? "RUNNING" : "IDLE";
      const title = item.annotations?.title || item.summary || "未命名会话";
      const wsUri = item.workspaceUris?.[0] || item.workspaces?.[0]?.workspaceFolderAbsoluteUri || "";
      const wsName = wsUri.split("/").filter(Boolean).pop() || "workspace";
      const timeStr = formatRelativeTime(item.lastModifiedTime);

      return `
        <div class="conv-card" onclick="navigateTo('#c=${item.id}')">
          <div class="conv-card-top">
            <div class="conv-title">${escapeHtml(title)}</div>
            <span class="badge ${badgeClass}">${badgeText}</span>
          </div>
          <div class="conv-card-bottom">
            <div class="conv-meta">
              <span>📁 ${escapeHtml(wsName)}</span>
              <span>•</span>
              <span>${item.stepCount || 0} 步骤</span>
            </div>
            <span>${timeStr}</span>
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

function updateChatControls(isRunning, wsUri) {
  const statusBadge = document.getElementById("chat-status-badge");
  const cancelBtn = document.getElementById("btn-cancel-task");
  const sendBtn = document.getElementById("btn-send");
  const wsTag = document.getElementById("chat-workspace-name");

  if (wsUri && wsTag) {
    const wsName = wsUri.split("/").filter(Boolean).pop() || "workspace";
    wsTag.textContent = "📁 " + wsName;
  }

  if (isRunning) {
    if (statusBadge) {
      statusBadge.className = "badge-running";
      statusBadge.textContent = "RUNNING";
    }
    if (cancelBtn) cancelBtn.classList.remove("hidden");
    if (sendBtn) {
      sendBtn.classList.add("btn-stop");
      sendBtn.title = "停止任务";
      sendBtn.setAttribute("aria-label", "停止任务");
      sendBtn.innerHTML = `
        <svg width="12" height="12" viewBox="0 0 12 12" fill="currentColor">
          <rect width="12" height="12" rx="2" fill="#ef4444" />
        </svg>
      `;
    }
  } else {
    if (statusBadge) {
      statusBadge.className = "badge-idle";
      statusBadge.textContent = "IDLE";
    }
    if (cancelBtn) cancelBtn.classList.add("hidden");
    if (sendBtn) {
      sendBtn.classList.remove("btn-stop");
      sendBtn.title = "发送";
      sendBtn.setAttribute("aria-label", "发送");
      sendBtn.innerHTML = `
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
          <line x1="22" y1="2" x2="11" y2="13"></line>
          <polygon points="22 2 15 22 11 13 2 9 22 2"></polygon>
        </svg>
      `;
    }
  }
}

function connectStreamWs(cascadeId) {
  closeActiveWs();

  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  const wsUrl = `${proto}//${location.host}/gateway/cascade/stream?cascadeId=${encodeURIComponent(cascadeId)}`;
  
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
        updateChatControls(isRunning, data.workspaceUri);

        if (currentTrajectories[cascadeId]) {
          currentTrajectories[cascadeId].status = data.status;
          currentTrajectories[cascadeId].stepCount = data.totalSteps;
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

    updateChatControls(isRunning, wsUri);
    renderMessages(steps, isRunning);

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

function getStepFingerprint(step) {
  if (!step) return "";
  const type = step.type || "";
  if (type === "CORTEX_STEP_TYPE_USER_INPUT") {
    const userText = step.userInput?.userResponse || step.userInput?.items?.[0]?.text || "";
    return `u:${userText.length}:${userText.slice(-10)}`;
  } else if (type === "CORTEX_STEP_TYPE_PLANNER_RESPONSE") {
    const p = step.plannerResponse || {};
    const thinkLen = (p.thinking || "").length;
    const respLen = (p.response || "").length;
    const lastChars = (p.response || "").slice(-12);
    return `p:${thinkLen}:${respLen}:${lastChars}`;
  } else {
    return `t:${type}:${step.status || ""}`;
  }
}

function generateStepHtml(step, i) {
  const type = step.type;

  if (type === "CORTEX_STEP_TYPE_USER_INPUT") {
    const userText = step.userInput?.userResponse || step.userInput?.items?.[0]?.text || "";
    return `<div class="bubble">${escapeHtml(userText)}</div>`;
  } else if (type === "CORTEX_STEP_TYPE_PLANNER_RESPONSE") {
    const p = step.plannerResponse || {};
    const thinking = p.thinking || "";
    const text = p.response || "";

    let thoughtHtml = "";
    if (thinking.trim()) {
      thoughtHtml = `
        <details class="thought-box">
          <summary>🧠 Agent 思考过程 (${thinking.length} 字符)</summary>
          <div class="thought-content">${escapeHtml(thinking)}</div>
        </details>
      `;
    }

    const bodyHtml = text ? getCachedMarkdown(text) : '<span style="color:var(--text-muted);">执行中...</span>';

    return `
      <div class="bubble markdown-body">
        ${thoughtHtml}
        <div>${bodyHtml}</div>
      </div>
    `;
  } else if (type && type.startsWith("CORTEX_STEP_TYPE_")) {
    const toolName = type.replace("CORTEX_STEP_TYPE_", "").toLowerCase();
    const status = step.status || "DONE";
    return `
      <details class="tool-box" style="width: 100%;">
        <summary>⚡ 工具调用: <strong>${escapeHtml(toolName)}</strong> <span style="font-size:11px;color:var(--text-muted);">(${escapeHtml(status)})</span></summary>
        <div class="tool-content">${escapeHtml(JSON.stringify(step, null, 2))}</div>
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

  // Remove any obsolete nodes if steps count decreased
  while (streamEl.children.length > steps.length) {
    streamEl.removeChild(streamEl.lastChild);
  }

  let hasDOMChanges = false;

  for (let i = 0; i < steps.length; i++) {
    const step = steps[i];
    const type = step.type;
    const fp = getStepFingerprint(step);
    const rowClass = type === "CORTEX_STEP_TYPE_USER_INPUT" ? "message-row user" : "message-row agent";

    let existingEl = document.getElementById(`step-item-${i}`);
    if (existingEl) {
      if (existingEl.getAttribute("data-fp") === fp) {
        // Unchanged: preserve DOM node completely
        continue;
      }
      // Content updated: patch in place
      existingEl.setAttribute("data-fp", fp);
      existingEl.className = rowClass;
      existingEl.innerHTML = generateStepHtml(step, i);
      hasDOMChanges = true;
    } else {
      // New step: create and append
      const newEl = document.createElement("div");
      newEl.id = `step-item-${i}`;
      newEl.className = rowClass;
      newEl.setAttribute("data-fp", fp);
      newEl.innerHTML = generateStepHtml(step, i);
      streamEl.appendChild(newEl);
      hasDOMChanges = true;
    }
  }

  // 1. First-time render on entering a conversation: align INSTANTLY with zero jitter
  if (!hasInitiallyAligned) {
    hasInitiallyAligned = true;
    prevWasRunning = isRunning;

    const lastStep = steps[steps.length - 1];
    if (!isRunning && lastStep && lastStep.type === "CORTEX_STEP_TYPE_PLANNER_RESPONSE") {
      let lastUserIdx = -1;
      for (let i = steps.length - 1; i >= 0; i--) {
        if (steps[i].type === "CORTEX_STEP_TYPE_USER_INPUT") {
          lastUserIdx = i;
          break;
        }
      }
      const turnStartIdx = lastUserIdx !== -1 ? lastUserIdx + 1 : 0;
      const turnStartEl = document.getElementById(`step-item-${turnStartIdx}`);
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
    const lastStep = steps[steps.length - 1];
    if (lastStep && lastStep.type === "CORTEX_STEP_TYPE_PLANNER_RESPONSE") {
      let lastUserIdx = -1;
      for (let i = steps.length - 1; i >= 0; i--) {
        if (steps[i].type === "CORTEX_STEP_TYPE_USER_INPUT") {
          lastUserIdx = i;
          break;
        }
      }
      const turnStartIdx = lastUserIdx !== -1 ? lastUserIdx + 1 : 0;
      const turnStartEl = document.getElementById(`step-item-${turnStartIdx}`);
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

// --- Send Message & Actions ---

async function sendMessage() {
  if (!activeCascadeId) return;

  const inputEl = document.getElementById("chat-input");
  const text = inputEl.value.trim();
  if (!text) return;

  inputEl.value = "";
  inputEl.style.height = "auto";

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
    }
    updateChatControls(true);

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
  }
}

async function cancelCurrentTask() {
  if (!activeCascadeId) return;
  if (!confirm("确定要终止当前 Agent 任务吗？")) return;

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

// --- New Conversation Modal ---

async function openNewModal() {
  const modal = document.getElementById("modal-new");
  modal.classList.remove("hidden");

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

function closeNewModal() {
  document.getElementById("modal-new").classList.add("hidden");
}

async function createConversation() {
  const ws = document.getElementById("new-workspace").value.trim();
  const model = document.getElementById("new-model").value;
  const prompt = document.getElementById("new-prompt").value.trim();

  if (!prompt) {
    alert("请输入首条指令");
    return;
  }

  const createBtn = document.getElementById("btn-modal-create");
  createBtn.textContent = "创建中...";
  createBtn.disabled = true;

  try {
    const startResp = await rpc("StartCascade", {
      workspaceUris: ws ? [ws.startsWith("file://") ? ws : "file://" + ws] : [],
      requestedModel: model || undefined
    });

    const cascadeId = startResp.cascadeId;
    if (!cascadeId) throw new Error("未返回新会话 ID");

    await rpc("SendUserCascadeMessage", {
      cascadeId: cascadeId,
      items: [{ text: prompt }]
    });

    closeNewModal();
    document.getElementById("new-prompt").value = "";
    navigateTo("#c=" + cascadeId);
  } catch (err) {
    alert("创建会话失败: " + err.message);
  } finally {
    createBtn.textContent = "开始执行";
    createBtn.disabled = false;
  }
}

// --- Helpers ---

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

function renderMarkdown(md) {
  if (!md) return "";
  md = processMathSymbols(md);
  let html = escapeHtml(md);

  // Fenced Code blocks
  html = html.replace(/```([a-zA-Z0-9_-]*)\n([\s\S]*?)```/g, (_, lang, code) => {
    return `<pre><code class="lang-${lang}">${code.trim()}</code></pre>`;
  });

  // Inline code
  html = html.replace(/`([^`]+)`/g, "<code>$1</code>");

  // Markdown links with file icon support
  html = html.replace(/(?<!\!)\[([^\]]+)\]\(([^)]+)\)/g, (_, text, url) => {
    const icon = resolveFileIcon(text) || resolveFileIcon(url);
    if (icon) {
      return `<a href="${url}" class="file-link" target="_blank" rel="noopener noreferrer"><img src="/icons/files/${icon}.svg" class="file-icon" alt="" /><span>${text}</span></a>`;
    }
    return `<a href="${url}" target="_blank" rel="noopener noreferrer">${text}</a>`;
  });

  // Bold & Italic
  html = html.replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>");
  html = html.replace(/\*([^*]+)\*/g, "<em>$1</em>");

  // Headers
  html = html.replace(/^### (.*$)/gim, "<h3>$1</h3>");
  html = html.replace(/^## (.*$)/gim, "<h2>$1</h2>");
  html = html.replace(/^# (.*$)/gim, "<h1>$1</h1>");

  // Lists
  html = html.replace(/^\s*-\s+(.*$)/gim, "<li>$1</li>");
  html = html.replace(/(<li>.*<\/li>)/gims, "<ul>$1</ul>");

  // Paragraphs
  html = html.split(/\n\n+/).map(p => {
    p = p.trim();
    if (p.startsWith("<h") || p.startsWith("<pre") || p.startsWith("<ul") || p.startsWith("<details") || p.startsWith("<a")) {
      return p;
    }
    return `<p>${p.replace(/\n/g, "<br/>")}</p>`;
  }).join("");

  return html;
}

// Markdown & LaTeX Parsing Memory Cache (LRU)
const markdownCache = new Map();
function getCachedMarkdown(md) {
  if (!md) return "";
  const len = md.length;
  const key = len + ":" + (len > 50 ? md.slice(0, 25) + ":" + md.slice(-25) : md);
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

  document.getElementById("btn-back").addEventListener("click", () => navigateTo("#"));
  document.getElementById("btn-new").addEventListener("click", openNewModal);
  document.getElementById("btn-refresh").addEventListener("click", () => {
    loadConversations();
    checkGatewayStatus();
  });
  document.getElementById("conn-pill").addEventListener("click", rescanGateway);
  document.getElementById("btn-close-modal").addEventListener("click", closeNewModal);
  document.getElementById("btn-modal-cancel").addEventListener("click", closeNewModal);
  document.getElementById("btn-modal-create").addEventListener("click", createConversation);
  document.getElementById("btn-send").addEventListener("click", () => {
    const summary = currentTrajectories[activeCascadeId];
    if (summary?.status === "CASCADE_RUN_STATUS_RUNNING") {
      cancelCurrentTask();
    } else {
      sendMessage();
    }
  });

  const searchInput = document.getElementById("conv-search");
  searchInput.addEventListener("input", () => renderConversationList(currentTrajectories));

  const chatInput = document.getElementById("chat-input");
  chatInput.addEventListener("input", () => {
    chatInput.style.height = "auto";
    chatInput.style.height = Math.min(chatInput.scrollHeight, 120) + "px";
  });
  chatInput.addEventListener("keydown", (e) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  });

  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible" && activeCascadeId) {
      if (!activeWs || activeWs.readyState !== WebSocket.OPEN) {
        connectStreamWs(activeCascadeId);
      }
    }
  });

  renderRoute();
});