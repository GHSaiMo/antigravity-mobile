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
  // The stream re-renders several times per second while an agent runs; each IndexedDB put
  // structured-clones the whole step list, so writes are coalesced per session (latest wins).
  pending: new Map(),
  flushTimer: null,
  WRITE_INTERVAL_MS: 1500,
  MAX_SESSIONS: 50,
  LRU_KEY: "agy_steps_cache_lru",

  schedule(cascadeId, data) {
    if (!cascadeId || !data) return;
    this.pending.set(cascadeId, data);
    if (!this.flushTimer) {
      this.flushTimer = setTimeout(() => this.flush(), this.WRITE_INTERVAL_MS);
    }
  },
  flush() {
    if (this.flushTimer) {
      clearTimeout(this.flushTimer);
      this.flushTimer = null;
    }
    const entries = Array.from(this.pending.entries());
    this.pending.clear();
    for (const [id, data] of entries) {
      this.set(id, data);
    }
  },
  // Keeps the store bounded: remembers write order in localStorage and drops the oldest sessions.
  touchLRU(cascadeId) {
    let ids = [];
    try {
      ids = JSON.parse(localStorage.getItem(this.LRU_KEY) || "[]");
    } catch (_) {}
    ids = ids.filter((id) => id !== cascadeId);
    ids.push(cascadeId);
    const evicted = ids.length > this.MAX_SESSIONS ? ids.splice(0, ids.length - this.MAX_SESSIONS) : [];
    try {
      localStorage.setItem(this.LRU_KEY, JSON.stringify(ids));
    } catch (_) {}
    for (const id of evicted) {
      if (id !== activeCascadeId) this.delete(id);
    }
  },

  async get(cascadeId) {
    if (!cascadeId) return null;
    if (this.pending.has(cascadeId)) return this.pending.get(cascadeId);
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
    this.touchLRU(cascadeId);
  },
  async delete(cascadeId) {
    if (!cascadeId) return;
    this.pending.delete(cascadeId);
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
  // Persist to IndexedDB (coalesced) for zero-latency instant restores
  PersistentStepsCache.schedule(cascadeId, data);
}

// Write any coalesced session snapshots before the page is hidden or unloaded.
document.addEventListener("visibilitychange", () => {
  if (document.visibilityState === "hidden") PersistentStepsCache.flush();
});
window.addEventListener("pagehide", () => PersistentStepsCache.flush());

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
let pendingImages = []; // [{ id, name, mimeType, base64Data, previewUrl }]

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

  const label = modelProviderLabel(activeModel);
  const name = modelDisplayName(activeModel);
  const target = isClaudeModel(activeModel) ? "Gemini" : "Claude";
  btn.className = `chip-pill chip-model-switch ${label === "Claude" ? "chip-claude" : "chip-gemini"}`;
  text.textContent = label;
  btn.title = `当前模型: ${name} - 点击切换为 ${target}`;
}

async function toggleModel() {
  activeModel = modelToggleTarget(activeModel);
  localStorage.setItem("agy_active_model", activeModel);
  updateModelSwitchUI();

  // Keep new-model dropdown in sync if opened
  const newModelSelect = document.getElementById("new-model");
  if (newModelSelect) {
    newModelSelect.value = activeModel;
  }

  // Update Language Server default model via JetboxWriteState
  try {
    await rpc("JetboxWriteState", {
      appState: {
        lastSelectedAgentModel: modelEnumFor(activeModel)
      }
    });
  } catch (err) {
    console.warn("[Model] Failed to sync model to Language Server:", err);
  }
}

function syncActiveModel(rawModel) {
  if (!rawModel) return;
  const target = resolveActiveModel(rawModel, activeModel);
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
      <img src="${img.previewUrl || img.dataUrl || ''}" alt="${escapeHtml(img.name || '图片')}" />
      <button class="image-preview-remove" type="button" aria-label="删除图片" onclick="removePendingImage('${img.id}')">✕</button>
    </div>
  `).join("");
  updateChatControls(currentTrajectories[activeCascadeId]?.status === "CASCADE_RUN_STATUS_RUNNING");
}

function removePendingImage(id) {
  const img = pendingImages.find(i => i.id === id);
  if (img && img.previewUrl) {
    try { URL.revokeObjectURL(img.previewUrl); } catch (_) {}
  }
  pendingImages = pendingImages.filter(img => img.id !== id);
  renderImagePreviews();
}

// Phone photos are 3-12 MB and go to the gateway as base64 (and are stored that way upstream), so
// shrink them like the iOS app does before sending: longest side 1600 px, JPEG.
const UPLOAD_IMAGE_MAX_DIM = 1600;
const UPLOAD_IMAGE_JPEG_QUALITY = 0.8;
const UPLOAD_IMAGE_KEEP_BELOW_BYTES = 1.5 * 1024 * 1024;

async function shrinkImageForUpload(file) {
  // Animated GIFs and vector images would lose what makes them what they are.
  if (file.type === "image/gif" || file.type === "image/svg+xml" || typeof createImageBitmap !== "function") {
    return file;
  }
  try {
    const bitmap = await createImageBitmap(file); // applies EXIF orientation
    const scale = Math.min(1, UPLOAD_IMAGE_MAX_DIM / Math.max(bitmap.width, bitmap.height));
    if (scale === 1 && file.size <= UPLOAD_IMAGE_KEEP_BELOW_BYTES) {
      bitmap.close();
      return file;
    }
    const canvas = document.createElement("canvas");
    canvas.width = Math.max(1, Math.round(bitmap.width * scale));
    canvas.height = Math.max(1, Math.round(bitmap.height * scale));
    const ctx = canvas.getContext("2d");
    ctx.fillStyle = "#fff"; // JPEG has no alpha: keep transparent PNGs readable
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    ctx.drawImage(bitmap, 0, 0, canvas.width, canvas.height);
    bitmap.close();
    const blob = await new Promise((resolve) => canvas.toBlob(resolve, "image/jpeg", UPLOAD_IMAGE_JPEG_QUALITY));
    return blob && blob.size < file.size ? blob : file;
  } catch (_) {
    return file;
  }
}

function handleFilesSelected(files) {
  if (!files || !files.length) return;
  for (const file of Array.from(files)) {
    if (!file.type.startsWith("image/")) continue;
    const id = `img-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
    const previewUrl = URL.createObjectURL(file);
    shrinkImageForUpload(file).then((upload) => {
      const reader = new FileReader();
      reader.onload = (e) => {
        const dataUrl = e.target.result;
        const base64Data = dataUrl.split(",")[1];
        pendingImages.push({
          id,
          name: file.name,
          mimeType: upload.type || file.type || "image/jpeg",
          base64Data,
          previewUrl
        });
        renderImagePreviews();
      };
      reader.readAsDataURL(upload);
    });
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
  if (versionEl) {
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


// ---------------------------------------------------------------------------
// 升级自检（网关 /gateway/status 的 compat 字段）
//
// Antigravity 升级后 language_server 的接口可能消失。网关读取其二进制里的方法清单，告诉客户端哪些
// 功能不可用；客户端隐藏对应入口，核心功能缺失时在首页顶部给出提示条。
// 原则与原生端一致：拿不到、解析不了、旧网关没有这个字段、网关无法检测时一律「放行」，宁可保留入口，
// 也不误隐藏可用功能。
// ---------------------------------------------------------------------------
const GATEWAY_FEATURE = {
  SEARCH: "search",
  EXPORT: "export",
  CHANGES: "changes",
  REVERT: "revert",
  SLASH: "slash",
  SUBAGENTS: "subagents",
};

let gatewayCompat = { version: "", checked: false, coreOk: true, unavailable: [] };
let lastCompatRefreshAt = 0;

function parseGatewayCompat(data) {
  const c = data && typeof data === "object" ? data.compat : null;
  if (!c || typeof c !== "object") return { version: "", checked: false, coreOk: true, unavailable: [] };
  return {
    version: typeof c.version === "string" ? c.version : "",
    checked: c.checked === true,
    coreOk: c.coreOk !== false,
    unavailable: Array.isArray(c.unavailable) ? c.unavailable.filter((x) => typeof x === "string") : [],
  };
}

function isFeatureAvailable(featureId) {
  return !(gatewayCompat.checked && gatewayCompat.unavailable.includes(featureId));
}

function isGatewayIncompatible() {
  return gatewayCompat.checked && !gatewayCompat.coreOk;
}

function gatewayCompatBannerText() {
  const ver = gatewayCompat.version ? `（${gatewayCompat.version}）` : "";
  return `当前 Antigravity 版本${ver} 与网关不兼容，部分基础功能可能无法使用。请升级网关（mgy）。`;
}

// 按自检结果隐藏带 data-feature 的入口，并刷新首页提示条。
// 不可用的功能记在 <html data-unavail="a b"> 上，由 CSS 属性选择器统一隐藏，
// 这样之后才动态生成的入口（如聊天气泡里的按钮）也自动生效。
function applyGatewayCompat() {
  const unavailable = gatewayCompat.checked ? gatewayCompat.unavailable : [];
  document.documentElement.setAttribute("data-unavail", unavailable.join(" "));
  const banner = document.getElementById("compat-banner");
  if (banner) {
    const text = document.getElementById("compat-banner-text");
    if (text) text.textContent = gatewayCompatBannerText();
    banner.classList.toggle("hidden", !isGatewayIncompatible());
  }
}

async function refreshGatewayCompat(force = false) {
  const now = Date.now();
  if (!force && now - lastCompatRefreshAt < 30000) return;
  lastCompatRefreshAt = now;
  try {
    const resp = await fetch("/gateway/status");
    if (!resp.ok) return; // 失败时保持上一次的结果
    gatewayCompat = parseGatewayCompat(await resp.json());
    applyGatewayCompat();
  } catch (_) {}
}

// 网关 JSON 接口（POST）。失败时抛出带服务端错误文案的 Error。
async function postGatewayJson(path, body) {
  const resp = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body || {}),
  });
  let data = null;
  try {
    data = await resp.json();
  } catch (_) {}
  if (!resp.ok) {
    throw new Error((data && (data.error || data.message)) || `请求失败 (HTTP ${resp.status})`);
  }
  return data || {};
}


// --- 视口适配（添加到主屏幕的独立模式）---
// 独立模式下 innerHeight 可能比真实屏幕矮一截（底部被截掉），按屏幕高度撑满 #app。
// 同时把关键数值写进设置页「视口」一行，便于在真机上对照排查。
function readSafeInsets() {
  const probe = document.createElement("div");
  probe.style.cssText = "position:fixed;visibility:hidden;pointer-events:none;top:env(safe-area-inset-top,0px);bottom:env(safe-area-inset-bottom,0px)";
  document.body.appendChild(probe);
  const r = probe.getBoundingClientRect();
  probe.remove();
  return { top: Math.round(r.top), bottom: Math.round(window.innerHeight - r.bottom) };
}

function syncAppViewport() {
  const standalone = navigator.standalone === true || window.matchMedia("(display-mode: standalone)").matches;
  const root = document.documentElement;
  const portraitPhone = Math.min(screen.width, screen.height) < 768 && window.innerHeight > window.innerWidth;
  // iOS 偶尔给独立模式的窗口比屏幕矮一截（实测差值恰好等于顶部安全区），超出部分系统直接裁掉，
  // 撑高页面没用。此时按实际窗口排版，并把底部安全区清零（Home 条在窗口之外），保证控件完整可见。
  const gap = Math.max(screen.width, screen.height) - window.innerHeight;
  const clipped = standalone && portraitPhone && gap > 20;
  root.toggleAttribute("data-clipped", clipped);
  const si = readSafeInsets();
  const el = document.getElementById("settings-viewport-info");
  if (el) {
    el.textContent = `${standalone ? "独立" : "浏览器"}${clipped ? "(窗口偏矮)" : ""} ${window.innerWidth}×${window.innerHeight} / 屏 ${screen.width}×${screen.height} / 安全区 ${si.top},${si.bottom}`;
  }
}

["resize", "orientationchange", "pageshow"].forEach((ev) => window.addEventListener(ev, syncAppViewport));
document.addEventListener("visibilitychange", () => {
  if (document.visibilityState === "visible") syncAppViewport();
});
window.addEventListener("DOMContentLoaded", syncAppViewport);
