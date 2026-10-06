// Antigravity Mobile Gateway - Web & PWA Client

// --- Global Auth Token Interceptor & 401 Handler (C-1) ---
// Web/PWA relies on HttpOnly Session Cookie (agy_dt) automatically managed by the browser.
// Secret tokens are not kept in localStorage to eliminate token theft via XSS.
const originalFetch = window.fetch;
window.fetch = async function (url, options = {}) {
  const response = await originalFetch(url, options);

  // Auto trigger pairing sheet if 401 Unauthorized encountered on protected API routes
  if (response.status === 401 && typeof url === "string" && !url.includes("/api/v1/auth/pair")) {
    console.warn("[Auth] 401 Unauthorized received for:", url);
    localStorage.removeItem("agy_paired");
    localStorage.removeItem("agy_device_id");
    localStorage.removeItem("agy_device_token");
    if (typeof updateAuthUI === "function") updateAuthUI();
    const inputEl = document.getElementById("input-pairing-code");
    const hasCode = inputEl && inputEl.value.trim().length > 0;
    if (!hasCode && typeof openPairingSheet === "function") {
      openPairingSheet("设备凭据已失效或被网关吊销，请重新配对");
    } else if (typeof openPairingSheet === "function") {
      openPairingSheet();
    }
  }

  return response;
};

// One-time purge of legacy token from localStorage
try {
  if (localStorage.getItem("agy_device_token")) {
    localStorage.setItem("agy_paired", "1");
    localStorage.removeItem("agy_device_token");
  }
} catch (_) {}

function isDevicePaired() {
  return localStorage.getItem("agy_paired") === "1" || !!localStorage.getItem("agy_device_id");
}

let activeCascadeId = null;
let pollTimer = null;
let currentTrajectories = {};
let availableModels = [];
let sessionStepsCache = {};
const MAX_SESSION_STEPS_CACHE = 30;
const sessionStepsLRU = [];

// --- IndexedDB Persistent Steps Cache (P1 / B-3) ---
const PersistentStepsCache = {
  dbPromise: null,
  getDB() {
    if (!this.dbPromise) {
      this.dbPromise = new Promise((resolve) => {
        if (typeof window === "undefined" || !window.indexedDB) return resolve(null);
        try {
          const req = indexedDB.open("agy_sessions_db", 1);
          req.onupgradeneeded = (e) => {
            const db = e.target.result;
            if (!db.objectStoreNames.contains("session_steps")) {
              db.createObjectStore("session_steps", { keyPath: "cascadeId" });
            }
          };
          req.onsuccess = () => resolve(req.result);
          req.onerror = () => resolve(null);
        } catch (_) {
          resolve(null);
        }
      });
    }
    return this.dbPromise;
  },
  async get(cascadeId) {
    if (!cascadeId) return null;
    const db = await this.getDB();
    if (!db) return null;
    return new Promise((resolve) => {
      try {
        const tx = db.transaction("session_steps", "readonly");
        const store = tx.objectStore("session_steps");
        const req = store.get(cascadeId);
        req.onsuccess = () => resolve(req.result?.data || null);
        req.onerror = () => resolve(null);
      } catch (_) {
        resolve(null);
      }
    });
  },
  async set(cascadeId, data) {
    if (!cascadeId || !data) return;
    const db = await this.getDB();
    if (!db) return;
    try {
      const tx = db.transaction("session_steps", "readwrite");
      const store = tx.objectStore("session_steps");
      store.put({ cascadeId, data, updatedAt: Date.now() });
    } catch (_) {}
  },
  async delete(cascadeId) {
    if (!cascadeId) return;
    const db = await this.getDB();
    if (!db) return;
    try {
      const tx = db.transaction("session_steps", "readwrite");
      const store = tx.objectStore("session_steps");
      store.delete(cascadeId);
    } catch (_) {}
  }
};

function setSessionStepsCache(cascadeId, data) {
  if (!cascadeId) return;
  sessionStepsCache[cascadeId] = data;
  const idx = sessionStepsLRU.indexOf(cascadeId);
  if (idx !== -1) sessionStepsLRU.splice(idx, 1);
  sessionStepsLRU.push(cascadeId);
  while (sessionStepsLRU.length > MAX_SESSION_STEPS_CACHE) {
    const oldest = sessionStepsLRU.shift();
    if (oldest && oldest !== activeCascadeId) {
      delete sessionStepsCache[oldest];
    }
  }
  // Asynchronously persist to IndexedDB for zero-latency instant restores
  PersistentStepsCache.set(cascadeId, data);
}

// --- Session Drafts Manager ---
const DraftManager = {
  get(cascadeId) {
    if (!cascadeId) return "";
    const drafts = this.getAll();
    return drafts[cascadeId] || "";
  },
  has(cascadeId) {
    if (!cascadeId) return false;
    return !!this.get(cascadeId).trim();
  },
  set(cascadeId, text) {
    if (!cascadeId) return;
    const drafts = this.getAll();
    if (text && text.trim()) {
      drafts[cascadeId] = text;
    } else {
      delete drafts[cascadeId];
    }
    try {
      localStorage.setItem("agy_session_drafts", JSON.stringify(drafts));
    } catch (_) {}
  },
  clear(cascadeId) {
    if (!cascadeId) return;
    const drafts = this.getAll();
    if (drafts[cascadeId]) {
      delete drafts[cascadeId];
      try {
        localStorage.setItem("agy_session_drafts", JSON.stringify(drafts));
      } catch (_) {}
    }
  },
  getAll() {
    try {
      return JSON.parse(localStorage.getItem("agy_session_drafts") || "{}");
    } catch (_) {
      return {};
    }
  }
};

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

  try {
    const resp = await fetch("/gateway/status");
    if (resp.status === 401) {
      throw new Error("unauthorized");
    }
    const data = await resp.json();

    if (data.status === "connected" && data.upstream) {
      if (statusPill) {
        statusPill.className = "status-badge connected";
        statusPill.textContent = "已连接";
      }
      if (portEl) portEl.textContent = data.upstream.port;
      if (pidEl) pidEl.textContent = data.upstream.pid;
    } else {
      if (statusPill) {
        statusPill.className = "status-badge disconnected";
        statusPill.textContent = "未连接";
      }
    }
  } catch (err) {
    if (statusPill) {
      statusPill.className = "status-badge disconnected";
      statusPill.textContent = "未连接";
    }
  }

  const versionEl = document.getElementById("settings-app-version");
  if (versionEl && !versionEl.textContent.trim()) {
    fetch("/api/v1/version")
      .then(r => r.json())
      .then(d => { if (d && d.version) versionEl.textContent = d.version; })
      .catch(() => {});
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
