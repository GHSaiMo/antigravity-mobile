// WeChat Chat Archive recognition, user attachment card rendering, and modal viewer

const WECHAT_ATTACHMENT_HEADER = "📎 附件（已上传到本机，可直接读取）：";
const WECHAT_AVATAR_PALETTE = [
  "#07C160",
  "#10AEFF",
  "#FA9D3B",
  "#6467F0",
  "#F4606C",
  "#19B5FE",
  "#A06CD5",
  "#2AC993"
];

function stripGatewayHexPrefix(fileName) {
  if (!fileName) return "";
  if (fileName.length > 17 && fileName[16] === "-" && /^[0-9a-f]{16}$/.test(fileName.slice(0, 16))) {
    return fileName.slice(17);
  }
  return fileName;
}

const WECHAT_DISPLAY_PREFIX_RE = /(?<![0-9a-fA-F])[0-9a-f]{16}-(聊天记录[^\s/`'"<>()\[\]，。；：、！？]*?\.zip)/gi;

function stripWeChatDisplayPrefix(text) {
  if (!text) return "";
  const s = String(text);
  if (!s.includes("聊天记录") || !s.includes("-")) return s;
  if (isWeChatChatArchiveName(s)) {
    const slashIdx = s.lastIndexOf("/");
    if (slashIdx >= 0) {
      const dir = s.slice(0, slashIdx + 1);
      const base = s.slice(slashIdx + 1);
      const stripped = stripGatewayHexPrefix(base);
      if (stripped !== base) return dir + stripped;
    } else {
      const stripped = stripGatewayHexPrefix(s);
      if (stripped !== s) return stripped;
    }
  }
  return s.replace(WECHAT_DISPLAY_PREFIX_RE, "$1");
}

function isWeChatChatArchiveName(pathOrName) {
  if (!pathOrName) return false;
  let s = String(pathOrName).trim().replace(/^[`'"]+|[`'"]+$/g, "");
  if (!s) return false;
  if (s.startsWith("file://")) s = s.slice(7);
  s = s.split("#")[0].split("?")[0];
  const rawBase = s.split("/").pop() || "";
  const base = stripGatewayHexPrefix(rawBase);
  const lower = base.toLowerCase();
  if (!lower.endsWith(".zip")) return false;
  if (/^聊天记录_\d{8}_\d{6}\.zip$/i.test(base)) return true;
  return base.startsWith("聊天记录");
}

function parseUserAttachmentBlock(text) {
  if (!text) return { body: "", files: [] };
  const idx = text.lastIndexOf(WECHAT_ATTACHMENT_HEADER);
  if (idx === -1) return { body: text, files: [] };

  const tail = text.slice(idx + WECHAT_ATTACHMENT_HEADER.length).trim();
  const files = [];
  const lineRe = /^- (.+) \(([^,()]*), ([^()]+)\)$/;

  for (const rawLine of tail.split("\n")) {
    const line = rawLine.trim();
    const m = lineRe.exec(line);
    if (!m) continue;
    const filePath = m[1];
    const rawName = filePath.split("/").pop() || filePath;
    const name = stripGatewayHexPrefix(rawName);
    const ext = (m[2] || "").trim();
    const sizeLabel = (m[3] || "").trim();
    files.push({
      path: filePath,
      name,
      ext,
      sizeLabel,
      isWeChat: isWeChatChatArchiveName(name)
    });
  }

  if (files.length === 0) return { body: text, files: [] };
  const body = text.slice(0, idx).trim();
  return { body, files };
}

function buildWeChatBadgeSvg() {
  return `<svg class="wechat-file-svg" viewBox="0 0 64 64" width="36" height="36" aria-hidden="true">
    <rect width="64" height="64" rx="14" fill="#07C160"/>
    <path d="M25.5 14C16.39 14 9 20.04 9 27.5c0 4.24 2.4 8.03 6.16 10.5l-1.54 4.65 5.4-2.7c2 .54 4.18.85 6.48.85 9.11 0 16.5-6.04 16.5-13.3C42 20.04 34.61 14 25.5 14z" fill="#FFFFFF"/>
    <circle cx="20" cy="24" r="1.9" fill="#07C160"/>
    <circle cx="31" cy="24" r="1.9" fill="#07C160"/>
    <path d="M41.2 28.5c-7.62 0-13.8 5.05-13.8 11.28s6.18 11.28 13.8 11.28c1.69 0 3.31-.25 4.81-.7l4.38 2.18-1.24-3.74c2.93-2.06 4.85-5.08 4.85-9.02 0-6.23-6.18-11.28-12.8-11.28z" fill="#FFFFFF" stroke="#07C160" stroke-width="2.6" stroke-linejoin="round"/>
    <circle cx="36.8" cy="36.8" r="1.6" fill="#07C160"/>
    <circle cx="45.6" cy="36.8" r="1.6" fill="#07C160"/>
  </svg>`;
}

function buildUserAttachmentCardsHtml(files) {
  if (!Array.isArray(files) || files.length === 0) return "";
  return `<div class="user-attachment-cards">` + files.map(f => {
    const extBadge = escapeHtml((f.ext || "FILE").toUpperCase().slice(0, 4));
    const badgeHtml = f.isWeChat
      ? `<div class="user-file-badge wechat">${buildWeChatBadgeSvg()}</div>`
      : `<div class="user-file-badge">${extBadge}</div>`;
    const typeLabel = f.isWeChat ? "微信聊天记录" : (f.ext || "FILE").toUpperCase();
    const sub = `${typeLabel} · ${f.sizeLabel}`;
    return `
      <button type="button" class="user-file-card" data-action="open-attachment-file" data-uri="${escapeHtml(f.path)}" data-name="${escapeHtml(f.name)}">
        ${badgeHtml}
        <div class="user-file-meta">
          <div class="user-file-name">${escapeHtml(f.name)}</div>
          <div class="user-file-sub">${escapeHtml(sub)}</div>
        </div>
      </button>
    `;
  }).join("") + `</div>`;
}

function buildZipEntryUrl(uri, entry, thumb = false) {
  const params = new URLSearchParams();
  params.set("uri", uri);
  params.set("entry", entry);
  if (typeof activeCascadeId !== "undefined" && activeCascadeId) {
    params.set("cascade_id", activeCascadeId);
  }
  if (thumb) params.set("thumb", "1");
  return `/api/v1/files/zip-entry?${params.toString()}`;
}

function formatWeChatBytes(bytes) {
  if (!bytes || bytes <= 0) return "";
  if (bytes >= 1048576) return `${(bytes / 1048576).toFixed(1)} MB`;
  if (bytes >= 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${bytes} B`;
}

const WeChatArchiveModal = (() => {
  let modalEl = null;
  let currentUri = "";
  let currentFileName = "";
  let currentArchive = null;
  let searchQuery = "";
  let isSearchOpen = false;

  function ensureModal() {
    if (modalEl) return modalEl;
    const div = document.createElement("div");
    div.id = "wechat-archive-modal";
    div.className = "wechat-archive-modal hidden";
    div.setAttribute("role", "dialog");
    div.setAttribute("aria-modal", "true");
    div.innerHTML = `
      <div class="wechat-archive-backdrop" data-wx-action="close"></div>
      <div class="wechat-archive-dialog">
        <div class="wechat-archive-header">
          <button type="button" class="wechat-nav-btn" data-wx-action="close" aria-label="关闭">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.3" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"></polyline></svg>
          </button>
          <div class="wechat-title-wrap">
            <div class="wechat-title" id="wx-archive-title">群聊的聊天记录</div>
            <div class="wechat-subtitle" id="wx-archive-subtitle"></div>
          </div>
          <div class="wechat-menu-wrap">
            <button type="button" class="wechat-nav-btn" data-wx-action="toggle-menu" aria-label="更多">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor"><circle cx="5" cy="12" r="1.8"/><circle cx="12" cy="12" r="1.8"/><circle cx="19" cy="12" r="1.8"/></svg>
            </button>
            <div class="wechat-dropdown hidden" id="wx-archive-menu">
              <button type="button" data-wx-action="toggle-search">搜索聊天记录</button>
              <button type="button" data-wx-action="copy-all">复制全部文本</button>
              <button type="button" data-wx-action="download-zip">下载原始 ZIP</button>
            </div>
          </div>
        </div>
        <div class="wechat-search-bar hidden" id="wx-archive-search-bar">
          <input type="search" id="wx-archive-search-input" placeholder="搜索群成员或聊天内容..." autocomplete="off" />
        </div>
        <div class="wechat-archive-body" id="wx-archive-body"></div>
      </div>
    `;
    document.body.appendChild(div);
    modalEl = div;

    div.addEventListener("click", (e) => {
      const actionEl = e.target.closest("[data-wx-action]");
      const menu = div.querySelector("#wx-archive-menu");
      if (!actionEl) {
        if (menu && !e.target.closest(".wechat-menu-wrap")) menu.classList.add("hidden");
        return;
      }
      const act = actionEl.getAttribute("data-wx-action");
      if (act === "close") {
        close();
      } else if (act === "toggle-menu") {
        menu?.classList.toggle("hidden");
      } else if (act === "toggle-search") {
        menu?.classList.add("hidden");
        isSearchOpen = !isSearchOpen;
        const bar = div.querySelector("#wx-archive-search-bar");
        if (bar) bar.classList.toggle("hidden", !isSearchOpen);
        if (isSearchOpen) {
          div.querySelector("#wx-archive-search-input")?.focus();
        } else {
          searchQuery = "";
          const inp = div.querySelector("#wx-archive-search-input");
          if (inp) inp.value = "";
          renderMessages();
        }
      } else if (act === "copy-all") {
        menu?.classList.add("hidden");
        if (currentArchive?.raw_text) {
          navigator.clipboard?.writeText(currentArchive.raw_text).then(() => {
            showMiniToast("已复制全部聊天记录");
          }).catch(() => {});
        }
      } else if (act === "download-zip") {
        menu?.classList.add("hidden");
        downloadRawZip();
      } else if (act === "open-wx-image") {
        const url = actionEl.getAttribute("data-url");
        const title = actionEl.getAttribute("data-title") || "图片";
        if (url && typeof ImageViewerManager !== "undefined") {
          ImageViewerManager.open(url, title, url);
        }
      } else if (act === "copy-msg") {
        const text = actionEl.getAttribute("data-copy");
        if (text) {
          navigator.clipboard?.writeText(text).then(() => {
            showMiniToast("已复制消息");
          }).catch(() => {});
        }
      }
    });

    div.querySelector("#wx-archive-search-input")?.addEventListener("input", (e) => {
      searchQuery = e.target.value || "";
      renderMessages();
    });

    return modalEl;
  }

  function showMiniToast(msg) {
    const el = ensureModal();
    let toast = el.querySelector(".wechat-mini-toast");
    if (!toast) {
      toast = document.createElement("div");
      toast.className = "wechat-mini-toast";
      el.querySelector(".wechat-archive-dialog")?.appendChild(toast);
    }
    toast.textContent = msg;
    toast.classList.add("show");
    clearTimeout(toast._timer);
    toast._timer = setTimeout(() => toast.classList.remove("show"), 1800);
  }

  function downloadRawZip() {
    if (!currentUri) return;
    const rawUrl = resolveMediaRawUrl(currentUri);
    const a = document.createElement("a");
    a.href = rawUrl;
    a.download = currentFileName || "聊天记录.zip";
    document.body.appendChild(a);
    a.click();
    a.remove();
  }

  function renderMentionText(text) {
    const escaped = escapeHtml(text || "");
    return escaped.replace(/(@[^\s\n&<]+)/g, '<span class="wx-mention">$1</span>').replace(/\n/g, "<br>");
  }

  function renderMessageContent(msg) {
    if (msg.kind === "image" && msg.media_entry) {
      const url = buildZipEntryUrl(currentUri, msg.media_entry, false);
      return `<img class="wx-msg-image" src="${escapeHtml(url)}" alt="${escapeHtml(msg.media_name || "图片")}" loading="lazy" data-wx-action="open-wx-image" data-url="${escapeHtml(url)}" data-title="${escapeHtml(msg.media_name || "图片")}" />`;
    }
    if (msg.kind === "video" && msg.media_entry) {
      const videoUrl = buildZipEntryUrl(currentUri, msg.media_entry, false);
      const posterUrl = buildZipEntryUrl(currentUri, msg.media_entry, true);
      const durBadge = msg.duration_label ? `<span class="wx-video-dur">${escapeHtml(msg.duration_label)}</span>` : "";
      return `
        <div class="wx-video-wrap">
          <video class="wx-msg-video" controls preload="metadata" playsinline poster="${escapeHtml(posterUrl)}" src="${escapeHtml(videoUrl)}"></video>
          ${durBadge}
        </div>
      `;
    }
    if (msg.kind === "missing_image") {
      return `<div class="wx-placeholder-pill">[图片] 未随记录导出</div>`;
    }
    if (msg.kind === "missing_video") {
      return `<div class="wx-placeholder-pill">[视频] 未随记录导出</div>`;
    }
    if (msg.kind === "sticker") {
      const label = msg.text ? `[动画表情] ${msg.text}` : "[动画表情]";
      return `<div class="wx-placeholder-pill">${escapeHtml(label)}</div>`;
    }
    if (msg.kind === "file" && msg.media_entry) {
      const fileUrl = buildZipEntryUrl(currentUri, msg.media_entry, false);
      const sz = formatWeChatBytes(msg.media_size);
      return `
        <a class="wx-file-card" href="${escapeHtml(fileUrl)}" target="_blank" rel="noopener">
          <div class="wx-file-name">${escapeHtml(msg.media_name || msg.text)}</div>
          ${sz ? `<div class="wx-file-size">${escapeHtml(sz)}</div>` : ""}
        </a>
      `;
    }
    return `<div class="wx-msg-text" data-wx-action="copy-msg" data-copy="${escapeHtml(msg.text || "")}" title="点击复制消息">${renderMentionText(msg.text || "")}</div>`;
  }

  function renderMessages() {
    const el = ensureModal();
    const bodyEl = el.querySelector("#wx-archive-body");
    if (!bodyEl || !currentArchive) return;

    const q = searchQuery.trim().toLowerCase();
    const list = (currentArchive.messages || []).filter(m => {
      if (!q) return true;
      return (m.sender || "").toLowerCase().includes(q) ||
        (m.text || "").toLowerCase().includes(q) ||
        (m.media_name || "").toLowerCase().includes(q);
    });

    if (list.length === 0) {
      bodyEl.innerHTML = `<div class="wx-empty">${q ? "没有匹配的聊天记录" : "聊天记录为空"}</div>`;
      return;
    }

    bodyEl.innerHTML = list.map(m => {
      const color = WECHAT_AVATAR_PALETTE[Math.abs(m.color_index || 0) % WECHAT_AVATAR_PALETTE.length];
      return `
        <div class="wx-msg-row">
          <div class="wx-avatar" style="background:${color};">${escapeHtml(m.sender_initial || "?")}</div>
          <div class="wx-msg-main">
            <div class="wx-msg-meta">
              <span class="wx-sender">${escapeHtml(m.sender || "")}</span>
              <span class="wx-time">${escapeHtml(m.display_time || "")}</span>
            </div>
            <div class="wx-msg-content">${renderMessageContent(m)}</div>
          </div>
        </div>
      `;
    }).join("");
  }

  async function open(uri, fileName) {
    const el = ensureModal();
    currentUri = uri;
    currentFileName = fileName || (uri.split("/").pop() || "聊天记录.zip");
    currentArchive = null;
    searchQuery = "";
    isSearchOpen = false;

    el.querySelector("#wx-archive-menu")?.classList.add("hidden");
    el.querySelector("#wx-archive-search-bar")?.classList.add("hidden");
    const inp = el.querySelector("#wx-archive-search-input");
    if (inp) inp.value = "";

    const titleEl = el.querySelector("#wx-archive-title");
    const subEl = el.querySelector("#wx-archive-subtitle");
    const bodyEl = el.querySelector("#wx-archive-body");
    if (titleEl) titleEl.textContent = "群聊的聊天记录";
    if (subEl) subEl.textContent = currentFileName;
    if (bodyEl) bodyEl.innerHTML = `<div class="wx-loading"><div class="ios-spinner"></div><span>正在解析微信聊天记录...</span></div>`;

    el.classList.remove("hidden");

    try {
      const params = new URLSearchParams();
      params.set("uri", uri);
      if (typeof activeCascadeId !== "undefined" && activeCascadeId) {
        params.set("cascade_id", activeCascadeId);
      }
      const resp = await fetch(`/api/v1/files/wechat-chat?${params.toString()}`);
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const data = await resp.json();
      if (!data.is_wechat_archive) {
        close();
        downloadRawZip();
        return;
      }
      currentArchive = data;
      if (titleEl) titleEl.textContent = data.title || "群聊的聊天记录";
      if (subEl) subEl.textContent = data.date_range || currentFileName;
      renderMessages();
    } catch (err) {
      if (bodyEl) {
        bodyEl.innerHTML = `
          <div class="wx-error">
            <div>解析失败：${escapeHtml(err.message)}</div>
            <button type="button" class="wx-retry-btn" data-wx-action="download-zip">下载原始 ZIP</button>
          </div>
        `;
      }
    }
  }

  function close() {
    if (!modalEl) return;
    modalEl.classList.add("hidden");
    modalEl.querySelectorAll("video").forEach(v => { try { v.pause(); } catch (_) {} });
  }

  return { open, close };
})();

window.isWeChatChatArchiveName = isWeChatChatArchiveName;
window.stripWeChatDisplayPrefix = stripWeChatDisplayPrefix;
window.parseUserAttachmentBlock = parseUserAttachmentBlock;
window.buildUserAttachmentCardsHtml = buildUserAttachmentCardsHtml;
window.WeChatArchiveModal = WeChatArchiveModal;
