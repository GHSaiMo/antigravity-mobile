// Antigravity 2.33.0 Feature Pack for Web & PWA
// Covers: Nord Theme & Color-Scheme toggle, R/Rmd syntax highlighting,
// GitHub PR pills + detail <dialog>, Workspace Trust banner, /btw side-questions,
// Find-in-Page (<dialog popover="manual"> + TreeWalker + CSS.highlights),
// and 200 MB Audio/Video/.amr & PDF/Code modal preview.

(function () {
  const MAX_GENERAL_UPLOAD_BYTES = 50 * 1024 * 1024;
  const MAX_MEDIA_UPLOAD_BYTES = 200 * 1024 * 1024;

  const AUDIO_EXTS = new Set(["mp3", "m4a", "wav", "aac", "flac", "ogg", "oga", "opus", "aif", "aiff", "wma", "amr", "caf"]);
  const VIDEO_EXTS = new Set(["mp4", "mov", "mkv", "webm", "avi", "flv", "wmv", "m4v", "3gp"]);
  const R_LANGS = new Set(["r", "rscript", "rmd", "rmarkdown", "rprofile", "renviron", "namespace"]);

  const PR_URL_RE = /https?:\/\/([^/\s]+)\/([A-Za-z0-9_.-]+)\/([A-Za-z0-9_.-]+)\/pull\/(\d+)(?:[/?#][^\s)\]"'<>]*)?/gi;

  const prCache = new Map();
  const prInFlight = new Set();
  const dismissedTrust = new Set();

  let currentFindRoot = null;
  let currentFindMatches = [];
  let currentFindIndex = -1;
  let fallbackMarks = [];

  let activeBtwParentId = "";
  let activeBtwStepIndex = null;
  let activeBtwPreview = "";
  let activeBtwQuote = "";
  const branchStateByCascade = new Map();
  const sharedExpandedByCascade = new Map();

  function fileExt(nameOrUrl) {
    const clean = String(nameOrUrl || "").split("?")[0].split("#")[0].toLowerCase();
    const dot = clean.lastIndexOf(".");
    return dot >= 0 ? clean.slice(dot + 1) : "";
  }

  function isAudioOrVideoFile(file) {
    const t = String(file?.type || "").toLowerCase();
    if (t.startsWith("audio/") || t.startsWith("video/")) return true;
    const ext = fileExt(file?.name);
    return AUDIO_EXTS.has(ext) || VIDEO_EXTS.has(ext);
  }

  function maxUploadBytesForFile(file) {
    return isAudioOrVideoFile(file) ? MAX_MEDIA_UPLOAD_BYTES : MAX_GENERAL_UPLOAD_BYTES;
  }

  // --- 1. Theme & Color Scheme (system | light | dark) ---
  const THEME_STORAGE_KEY = "antigravity_theme";
  let activeSettingsSubpage = null; // null | "compact"

  function getNormalizedThemeMode() {
    try {
      let mode = localStorage.getItem(THEME_STORAGE_KEY) || "system";
      if (mode === "nord") {
        mode = "dark";
        localStorage.setItem(THEME_STORAGE_KEY, "dark");
      }
      return ["system", "light", "dark"].includes(mode) ? mode : "system";
    } catch (_) {
      return "system";
    }
  }

  function updateThemeSegmentedUI(mode) {
    document.querySelectorAll("#settings-theme-mode-seg .settings-seg-btn").forEach((btn) => {
      const active = btn.getAttribute("data-theme-mode") === mode;
      btn.classList.toggle("active", active);
      btn.setAttribute("aria-pressed", active ? "true" : "false");
    });
  }

  function openSettingsSubpage(kind) {
    activeSettingsSubpage = kind === "compact" || kind === "alerts" ? kind : null;
    const headerMain = document.getElementById("settings-header-main");
    const headerSub = document.getElementById("settings-header-sub");
    const mainBody = document.getElementById("settings-main-body");
    const subBody = document.getElementById("settings-sub-body");
    const subCompact = document.getElementById("settings-sub-compact");
    const subAlerts = document.getElementById("settings-sub-alerts");
    const subTitle = document.getElementById("settings-sub-title");

    subCompact?.classList.add("hidden");
    subAlerts?.classList.add("hidden");

    if (!activeSettingsSubpage) {
      headerMain?.classList.remove("hidden");
      headerSub?.classList.add("hidden");
      mainBody?.classList.remove("hidden");
      subBody?.classList.add("hidden");
      return;
    }

    headerMain?.classList.add("hidden");
    headerSub?.classList.remove("hidden");
    mainBody?.classList.add("hidden");
    subBody?.classList.remove("hidden");
    if (subBody) subBody.scrollTop = 0;

    if (activeSettingsSubpage === "alerts") {
      if (subTitle) subTitle.textContent = typeof t === "function" ? t("提醒内容") : "提醒内容";
      subAlerts?.classList.remove("hidden");
      if (window.PushAlertPrefs?.refresh) window.PushAlertPrefs.refresh();
    } else {
      if (subTitle) subTitle.textContent = typeof t === "function" ? t("精简模式") : "精简模式";
      subCompact?.classList.remove("hidden");
    }
  }

  function applyTheme(mode) {
    const theme = mode === "nord"
      ? "dark"
      : (["system", "light", "dark"].includes(mode) ? mode : getNormalizedThemeMode());
    try {
      localStorage.setItem(THEME_STORAGE_KEY, theme);
    } catch (_) {}

    const root = document.documentElement;
    const csMeta = document.querySelector('meta[name="color-scheme"]');
    const tcMeta = document.getElementById("meta-theme-color");

    if (theme === "system") {
      root.removeAttribute("data-theme");
      if (csMeta) csMeta.setAttribute("content", "light dark");
      if (tcMeta) {
        const isLight = window.matchMedia && window.matchMedia("(prefers-color-scheme: light)").matches;
        tcMeta.setAttribute("content", isLight ? "#f2f2f7" : "#0f172a");
      }
    } else {
      root.setAttribute("data-theme", theme);
      if (csMeta) csMeta.setAttribute("content", theme);
      if (tcMeta) tcMeta.setAttribute("content", theme === "light" ? "#f2f2f7" : "#0f172a");
    }
    updateThemeSegmentedUI(theme);
    if (typeof window.isWebDarkTheme === "function" && typeof window.buildAntigravityThemeCss === "function") {
      const isDark = window.isWebDarkTheme();
      const payload = {
        type: "antigravity:themechange",
        isDark,
        cssInline: window.buildAntigravityThemeCss(isDark, "inline", true),
        cssStandalone: window.buildAntigravityThemeCss(isDark, "standalone", true),
      };
      document.querySelectorAll("iframe.agent-embed-frame, iframe.html-artifact-preview-frame").forEach((frame) => {
        try {
          frame.contentWindow?.postMessage(payload, "*");
        } catch (_) {}
      });
    }
  }

  function initThemeSettings() {
    applyTheme();
    document.querySelectorAll("#settings-theme-mode-seg .settings-seg-btn").forEach((btn) => {
      btn.addEventListener("click", () => {
        const mode = btn.getAttribute("data-theme-mode");
        if (mode) applyTheme(mode);
      });
    });
    document.getElementById("btn-open-compact-settings")?.addEventListener("click", () => openSettingsSubpage("compact"));
    document.getElementById("btn-open-alerts-settings")?.addEventListener("click", () => openSettingsSubpage("alerts"));
    document.getElementById("btn-settings-sub-back")?.addEventListener("click", () => openSettingsSubpage(null));
    document.getElementById("btn-sheet-settings-close-sub")?.addEventListener("click", () => {
      openSettingsSubpage(null);
      if (typeof closeSettingsSheet === "function") closeSettingsSheet();
    });
    window.addEventListener("storage", (e) => {
      if (e.key === THEME_STORAGE_KEY) applyTheme();
    });
  }

  // --- 2. R & R Markdown Syntax Highlighting ---
  const R_KEYWORDS = new Set([
    "if", "else", "repeat", "while", "function", "for", "in", "next", "break",
    "TRUE", "FALSE", "NULL", "Inf", "NaN", "NA", "NA_integer_", "NA_real_",
    "NA_complex_", "NA_character_", "library", "require", "source", "return",
    "switch", "tryCatch", "stop", "warning", "message", "local", "with"
  ]);

  function escapeHtmlLocal(s) {
    return String(s || "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function highlightRCode(rawCode) {
    const tokenRe = /(#[^\n]*)|("(?:\\[\s\S]|[^"\\])*"|'(?:\\[\s\S]|[^'\\])*'|`[^`\n]+`)|(<<-|->|->>|<-|%>%|\|>|%[A-Za-z0-9_.]+%|:::?|\b\d+(?:\.\d+)?(?:[eE][+-]?\d+)?[Li]?\b|\b(?:0x[0-9a-fA-F]+)\b)|(\b[A-Za-z_.][A-Za-z0-9_.]*)(?=\s*\()|(\b[A-Za-z_.][A-Za-z0-9_.]*\b)/g;
    let out = "";
    let last = 0;
    let m;
    while ((m = tokenRe.exec(rawCode)) !== null) {
      if (m.index > last) out += escapeHtmlLocal(rawCode.slice(last, m.index));
      const [full, cmt, str, opOrNum, fnName, ident] = m;
      if (cmt) {
        out += `<span style="color:var(--tok-cmt);">${escapeHtmlLocal(cmt)}</span>`;
      } else if (str) {
        out += `<span style="color:var(--tok-str);">${escapeHtmlLocal(str)}</span>`;
      } else if (opOrNum) {
        const isOp = opOrNum.startsWith("<") || opOrNum.startsWith("-") || opOrNum.startsWith("|") || opOrNum.startsWith("%") || opOrNum.startsWith(":");
        const color = isOp ? "var(--tok-kw)" : "var(--tok-num)";
        out += `<span style="color:${color};">${escapeHtmlLocal(opOrNum)}</span>`;
      } else if (fnName) {
        const color = R_KEYWORDS.has(fnName) ? "var(--tok-kw)" : "var(--tok-fn)";
        out += `<span style="color:${color};">${escapeHtmlLocal(fnName)}</span>`;
      } else if (ident) {
        if (R_KEYWORDS.has(ident)) {
          out += `<span style="color:var(--tok-kw);font-weight:600;">${escapeHtmlLocal(ident)}</span>`;
        } else {
          out += escapeHtmlLocal(ident);
        }
      } else {
        out += escapeHtmlLocal(full);
      }
      last = tokenRe.lastIndex;
    }
    if (last < rawCode.length) out += escapeHtmlLocal(rawCode.slice(last));
    return out;
  }

  function highlightRmdCode(rawCode) {
    const lines = String(rawCode || "").split("\n");
    let inChunk = false;
    let inYaml = false;
    const out = [];
    for (let i = 0; i < lines.length; i++) {
      const line = lines[i];
      const trimmed = line.trim();
      if (i === 0 && trimmed === "---") {
        inYaml = true;
        out.push(`<span style="color:var(--tok-meta);">${escapeHtmlLocal(line)}</span>`);
        continue;
      }
      if (inYaml) {
        if (trimmed === "---" || trimmed === "...") {
          inYaml = false;
          out.push(`<span style="color:var(--tok-meta);">${escapeHtmlLocal(line)}</span>`);
        } else {
          out.push(`<span style="color:var(--tok-attr);">${escapeHtmlLocal(line)}</span>`);
        }
        continue;
      }
      if (/^```\{[rR]\b[^}]*\}\s*$/.test(trimmed)) {
        inChunk = true;
        out.push(`<span style="color:var(--tok-meta);font-weight:600;">${escapeHtmlLocal(line)}</span>`);
        continue;
      }
      if (inChunk && /^```\s*$/.test(trimmed)) {
        inChunk = false;
        out.push(`<span style="color:var(--tok-meta);font-weight:600;">${escapeHtmlLocal(line)}</span>`);
        continue;
      }
      if (inChunk) {
        out.push(highlightRCode(line));
      } else if (/^#{1,6}\s/.test(trimmed)) {
        out.push(`<span style="color:var(--tok-kw);font-weight:700;">${escapeHtmlLocal(line)}</span>`);
      } else {
        out.push(escapeHtmlLocal(line));
      }
    }
    return out.join("\n");
  }

  function installRHighlighter() {
    if (typeof window.highlightCodeHtml !== "function" || window.highlightCodeHtml.__v233Wrapped) return;
    const orig = window.highlightCodeHtml;
    const wrapped = function (rawCode, langClean) {
      const lang = String(langClean || "").trim().toLowerCase();
      if (lang === "rmd" || lang === "rmarkdown") {
        return highlightRmdCode(rawCode);
      }
      if (R_LANGS.has(lang)) {
        return highlightRCode(rawCode);
      }
      return orig(rawCode, langClean);
    };
    wrapped.__v233Wrapped = true;
    window.highlightCodeHtml = wrapped;
    if (typeof highlightCodeHtml === "function") {
      try {
        highlightCodeHtml = wrapped;
      } catch (_) {}
    }
  }

  // --- 3. GitHub Pull Request Pills & Detail <dialog> ---
  function normalizePrUrl(raw) {
    PR_URL_RE.lastIndex = 0;
    const m = PR_URL_RE.exec(String(raw || ""));
    if (!m) return "";
    return `https://${m[1].toLowerCase()}/${m[2]}/${m[3]}/pull/${m[4]}`;
  }

  function prStateTone(state) {
    const s = String(state || "OPEN").toUpperCase();
    if (s === "MERGED") return "merged";
    if (s === "DRAFT") return "draft";
    if (s === "CLOSED") return "closed";
    return "open";
  }

  function renderPrPillsForBubble(bubble, urls) {
    const resolved = urls.map((u) => prCache.get(u)).filter(Boolean);
    if (!resolved.length) return;
    let row = bubble.querySelector(".v233-pr-pills-row");
    const sig = resolved.map((p) => `${p.url}:${p.state}:${p.title}`).join("|");
    if (row && row.getAttribute("data-pr-sig") === sig) return;
    if (!row) {
      row = document.createElement("div");
      row.className = "v233-pr-pills-row";
      bubble.appendChild(row);
    }
    row.setAttribute("data-pr-sig", sig);
    row.innerHTML = resolved
      .map((pr) => {
        const st = String(pr.state || "OPEN").toUpperCase();
        const tone = prStateTone(st);
        const key = encodeURIComponent(pr.url || "");
        return `<button type="button" class="v233-pr-pill" data-pr-url="${key}" title="${escapeHtmlLocal(pr.title || "")}">
          <span class="git-pr-badge ${tone}">${escapeHtmlLocal(st)}</span>
          <span class="git-pr-num">#${Number(pr.number) || ""}</span>
          <span class="v233-pr-pill-title">${escapeHtmlLocal(pr.title || `${pr.owner}/${pr.repo}`)}</span>
        </button>`;
      })
      .join("");
  }

  async function hydrateChatPRPills(containerEl) {
    const root = containerEl || document.getElementById("messages-stream");
    if (!root) return;
    const bubbles = Array.from(root.querySelectorAll(".bubble"));
    const urlsToFetch = [];

    for (const bubble of bubbles) {
      const links = Array.from(bubble.querySelectorAll("a[href]"));
      const prUrls = [];
      for (const a of links) {
        const norm = normalizePrUrl(a.getAttribute("href"));
        if (norm && !prUrls.includes(norm)) {
          prUrls.push(norm);
        }
      }
      if (!prUrls.length) continue;
      renderPrPillsForBubble(bubble, prUrls);
      for (const u of prUrls) {
        if (!prCache.has(u) && !prInFlight.has(u)) {
          prInFlight.add(u);
          urlsToFetch.push(u);
        }
      }
    }

    if (!urlsToFetch.length) return;
    try {
      const res = await fetch("/gateway/github/pull-requests", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ urls: urlsToFetch.slice(0, 12) }),
      });
      if (!res.ok) return;
      const data = await res.json();
      const list = Array.isArray(data.pullRequests) ? data.pullRequests : [];
      for (const pr of list) {
        const norm = normalizePrUrl(pr.url);
        if (norm) prCache.set(norm, { ...pr, url: norm });
      }
      // Re-pass to mount pills now that cache is warm
      for (const bubble of Array.from((document.getElementById("messages-stream") || root).querySelectorAll(".bubble"))) {
        const prUrls = [];
        for (const a of bubble.querySelectorAll("a[href]")) {
          const norm = normalizePrUrl(a.getAttribute("href"));
          if (norm && !prUrls.includes(norm)) prUrls.push(norm);
        }
        if (prUrls.length) renderPrPillsForBubble(bubble, prUrls);
      }
    } catch (_) {
    } finally {
      urlsToFetch.forEach((u) => prInFlight.delete(u));
    }
  }

  function openPRDetailModal(pr) {
    const dlg = document.getElementById("dialog-github-pr");
    if (!dlg || !pr) return;
    const stateEl = document.getElementById("pr-dialog-state");
    const titleEl = document.getElementById("pr-dialog-title");
    const bodyEl = document.getElementById("pr-dialog-body");
    const extEl = document.getElementById("pr-dialog-open-ext");
    const askBtn = document.getElementById("btn-pr-dialog-ask");

    const st = String(pr.state || "OPEN").toUpperCase();
    if (stateEl) {
      stateEl.className = `git-pr-badge ${prStateTone(st)}`;
      stateEl.textContent = st;
    }
    if (titleEl) {
      titleEl.textContent = `#${Number(pr.number) || ""} ${pr.title || ""}`.trim();
    }
    if (extEl) {
      extEl.href = pr.url || "#";
    }
    if (bodyEl) {
      const repoLabel = pr.owner && pr.repo ? `${pr.owner}/${pr.repo}` : "";
      const refLabel = pr.headRefName && pr.baseRefName ? `${pr.headRefName} → ${pr.baseRefName}` : "";
      const author = pr.authorLogin ? `@${pr.authorLogin}` : "";
      const bodyText = String(pr.body || "").trim();
      bodyEl.innerHTML = `
        <div class="v233-pr-meta-row">
          ${repoLabel ? `<span>📦 ${escapeHtmlLocal(repoLabel)}</span>` : ""}
          ${author ? `<span>👤 ${escapeHtmlLocal(author)}</span>` : ""}
          ${refLabel ? `<span>🌿 ${escapeHtmlLocal(refLabel)}</span>` : ""}
        </div>
        ${bodyText ? `<div class="v233-pr-body-pre">${escapeHtmlLocal(bodyText)}</div>` : `<div style="color:var(--ios-secondary-label);">无 Pull Request 描述正文</div>`}
      `;
    }
    if (askBtn) {
      askBtn.onclick = () => {
        dlg.close();
        const quote = `PR #${pr.number || ""} (${pr.state || "OPEN"}): ${pr.title || ""}\n${pr.url || ""}`;
        openBtwModal({ parentCascadeId: activeCascadeId || "", quote });
      };
    }
    if (typeof dlg.showModal === "function") dlg.showModal();
  }

  // --- 4. Workspace Trust Gate ---
  let currentTrustPayload = null;

  async function checkWorkspaceTrust(cascadeId, workspaceUri) {
    const banner = document.getElementById("workspace-trust-banner");
    if (!banner) return;

    let ws = String(workspaceUri || "").trim();
    if (!ws && cascadeId && typeof currentTrajectories === "object") {
      ws = currentTrajectories[cascadeId]?.workspaceUris?.[0] || "";
    }
    if (!ws && typeof activeDraftSession === "object" && activeDraftSession && !activeDraftSession.isPure) {
      ws = activeDraftSession.uri || activeDraftSession.path || "";
    }
    if (!ws || ws === "outside-of-project" || dismissedTrust.has(ws)) {
      banner.classList.add("hidden");
      currentTrustPayload = null;
      return;
    }

    try {
      const qs = new URLSearchParams();
      if (cascadeId) qs.set("cascadeId", cascadeId);
      if (ws) qs.set("workspaceUri", ws);
      const res = await fetch(`/gateway/projects/trust?${qs.toString()}`);
      if (!res.ok) return;
      const st = await res.json();
      if (st && st.ok && (st.isProjectTrusted === false || st.hasBlockedItems === true)) {
        currentTrustPayload = { cascadeId, workspaceUri: ws, projectId: st.projectId || "" };
        const desc = document.getElementById("wtb-workspace-path");
        if (desc) desc.textContent = ws.replace(/^file:\/\//, "");
        banner.classList.remove("hidden");
      } else {
        currentTrustPayload = null;
        banner.classList.add("hidden");
      }
    } catch (_) {
      banner.classList.add("hidden");
    }
  }

  function initTrustBanner() {
    document.getElementById("btn-dismiss-trust")?.addEventListener("click", () => {
      if (currentTrustPayload?.workspaceUri) dismissedTrust.add(currentTrustPayload.workspaceUri);
      document.getElementById("workspace-trust-banner")?.classList.add("hidden");
    });

    document.getElementById("btn-trust-workspace")?.addEventListener("click", async () => {
      if (!currentTrustPayload) return;
      const btn = document.getElementById("btn-trust-workspace");
      if (btn) {
        btn.disabled = true;
        btn.textContent = "授权中…";
      }
      try {
        const res = await fetch("/gateway/projects/trust", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ ...currentTrustPayload, trust: true }),
        });
        const d = await res.json().catch(() => ({}));
        if (!res.ok || d.ok === false) throw new Error(d.error || "设置失败");
        document.getElementById("workspace-trust-banner")?.classList.add("hidden");
        currentTrustPayload = null;
        window.showToast?.("已信任当前工作区");
      } catch (err) {
        window.showToast?.("信任设置失败: " + (err.message || "未知错误"), 2800);
      } finally {
        if (btn) {
          btn.disabled = false;
          btn.textContent = "信任工作区";
        }
      }
    });
  }

  // --- 5. /btw Side-Question Branch Architecture ---
  function getBranchState(cascadeId) {
    return (cascadeId && branchStateByCascade.get(cascadeId)) || { branchOrigin: null, sideQuestions: [] };
  }

  function syncBranchState(cascadeId, payload) {
    if (!cascadeId || !payload) return;
    const prev = getBranchState(cascadeId);
    const nextOrigin = payload.branchOrigin !== undefined ? (payload.branchOrigin || null) : prev.branchOrigin;
    const nextSides = Array.isArray(payload.sideQuestions) ? payload.sideQuestions : [];
    branchStateByCascade.set(cascadeId, { branchOrigin: nextOrigin, sideQuestions: nextSides });
    if (typeof activeCascadeId !== "undefined" && activeCascadeId === cascadeId) {
      applyBranchDecorations(document.getElementById("messages-stream"), cascadeId);
    }
  }

  function ensureBranchOriginBanner(streamEl) {
    let banner = document.getElementById("btw-branch-origin-banner");
    if (!banner && streamEl && streamEl.parentNode) {
      banner = document.createElement("div");
      banner.id = "btw-branch-origin-banner";
      banner.className = "btw-branch-origin-banner hidden";
      streamEl.parentNode.insertBefore(banner, streamEl);
    }
    return banner;
  }

  function insertBranchConclusionToParent(parentId, rawText) {
    const pid = String(parentId || "").trim();
    const snippet = String(rawText || "").trim().slice(0, 1600);
    if (!pid || !snippet) return;
    if (typeof DraftManager !== "undefined") {
      const existing = String(DraftManager.get(pid) || "").trim();
      DraftManager.set(pid, existing ? `${existing}\n\n${snippet}` : snippet);
    }
    if (typeof navigateTo === "function") {
      navigateTo("#c=" + pid);
      setTimeout(() => {
        const chatInput = document.getElementById("chat-input");
        if (chatInput) {
          chatInput.focus();
          chatInput.style.height = "auto";
          chatInput.style.height = Math.min(chatInput.scrollHeight, 120) + "px";
        }
      }, 120);
    }
  }

  function applyBranchDecorations(streamEl, cascadeId) {
    const cid = cascadeId || (typeof activeCascadeId !== "undefined" ? activeCascadeId : "");
    const banner = ensureBranchOriginBanner(streamEl);
    if (!streamEl || !cid) {
      banner?.classList.add("hidden");
      return;
    }

    const { branchOrigin, sideQuestions } = getBranchState(cid);
    const rows = Array.from(streamEl.children).filter((el) => el.classList?.contains("message-row"));

    // 1. Branched chat view: origin banner + collapsed shared prefix + one-tap insert to parent
    if (banner) {
      if (branchOrigin && branchOrigin.parentCascadeId) {
        const expanded = !!sharedExpandedByCascade.get(cid);
        const parentTitle = String(branchOrigin.parentTitle || "").trim();
        const turnNum = Number(branchOrigin.forkTurnNumber || 0);
        const originLabel = parentTitle && turnNum > 0
          ? `承接自「${parentTitle}」第 ${turnNum} 轮`
          : parentTitle
          ? `承接自「${parentTitle}」`
          : turnNum > 0
          ? `承接自主会话第 ${turnNum} 轮`
          : "承接自主会话上下文";
        const sig = `${branchOrigin.parentCascadeId}|${originLabel}|${expanded ? 1 : 0}`;
        if (banner.getAttribute("data-sig") !== sig || banner.classList.contains("hidden")) {
          banner.setAttribute("data-sig", sig);
          banner.classList.remove("hidden");
          banner.innerHTML = `
            <div class="btw-origin-main">
              <span class="btw-origin-icon">⑂</span>
              <div class="btw-origin-text">
                <div class="btw-origin-title">${escapeHtmlLocal(originLabel)}</div>
                <div class="btw-origin-sub">${expanded ? "已展开共享前文" : "已共享前文上下文（默认折叠）"}</div>
              </div>
            </div>
            <div class="btw-origin-actions">
              <button type="button" class="btw-origin-btn accent" data-btw-action="toggle-shared">${expanded ? "收起前文" : "展开前文"}</button>
              <button type="button" class="btw-origin-btn" data-btw-action="return-parent">回主会话</button>
            </div>
          `;
          banner.querySelector('[data-btw-action="toggle-shared"]')?.addEventListener("click", () => {
            sharedExpandedByCascade.set(cid, !sharedExpandedByCascade.get(cid));
            applyBranchDecorations(streamEl, cid);
          });
          banner.querySelector('[data-btw-action="return-parent"]')?.addEventListener("click", () => {
            if (typeof navigateTo === "function") navigateTo("#c=" + branchOrigin.parentCascadeId);
          });
        }
      } else {
        banner.classList.add("hidden");
      }
    }

    const forkStep = branchOrigin ? Number(branchOrigin.forkStepIndex) : -1;
    const isExpanded = branchOrigin ? !!sharedExpandedByCascade.get(cid) : false;
    let firstPostForkMarked = false;

    for (const row of rows) {
      const rawStep = row.getAttribute("data-step-index");
      const stepIdx = rawStep !== null && rawStep !== "" && !Number.isNaN(Number(rawStep)) ? Number(rawStep) : null;
      const isSharedPrefix = !!(branchOrigin && stepIdx !== null && stepIdx <= forkStep);

      row.classList.toggle("branch-shared-hidden", isSharedPrefix && !isExpanded);
      row.classList.toggle("branch-shared-prefix", isSharedPrefix && isExpanded);

      const markDivider = !!(branchOrigin && isExpanded && !isSharedPrefix && !firstPostForkMarked);
      if (markDivider) firstPostForkMarked = true;
      row.classList.toggle("branch-fork-divider-before", markDivider);

      const existingInsertBar = row.querySelector(":scope > .btw-insert-parent-bar");
      const shouldHaveInsert = !!(branchOrigin && branchOrigin.parentCascadeId && !isSharedPrefix && row.classList.contains("agent") && !row.classList.contains("error-row"));
      if (shouldHaveInsert) {
        if (!existingInsertBar) {
          const bar = document.createElement("div");
          bar.className = "btw-insert-parent-bar";
          bar.innerHTML = `<button type="button" class="btw-insert-parent-btn">↵ 插入主对话</button>`;
          bar.querySelector("button")?.addEventListener("click", (e) => {
            e.stopPropagation();
            const body = row.querySelector(".agent-message-body") || row.querySelector(".bubble");
            const txt = body ? body.innerText.trim() : "";
            insertBranchConclusionToParent(branchOrigin.parentCascadeId, txt);
          });
          row.appendChild(bar);
        }
      } else if (existingInsertBar) {
        existingInsertBar.remove();
      }
    }

    // 2. Parent conversation inline branch cards anchored beneath source messages
    const cardsByRowId = new Map();
    if (Array.isArray(sideQuestions) && sideQuestions.length > 0 && rows.length > 0) {
      const rowById = new Map();
      let firstLoadedStep = Infinity;
      for (const r of rows) {
        if (r.id) rowById.set(r.id, r);
        const raw = r.getAttribute("data-step-index");
        if (raw !== null && raw !== "" && !Number.isNaN(Number(raw))) {
          firstLoadedStep = Math.min(firstLoadedStep, Number(raw));
        }
      }
      if (!Number.isFinite(firstLoadedStep)) firstLoadedStep = 0;

      for (const card of sideQuestions) {
        let targetRow = card.anchorMessageId ? rowById.get(card.anchorMessageId) : null;
        if (!targetRow && typeof card.forkStepIndex === "number" && card.forkStepIndex >= firstLoadedStep) {
          for (let i = rows.length - 1; i >= 0; i--) {
            const raw = rows[i].getAttribute("data-step-index");
            const si = raw !== null && raw !== "" && !Number.isNaN(Number(raw)) ? Number(raw) : null;
            if (si !== null && si <= card.forkStepIndex && (rows[i].classList.contains("agent") || rows[i].classList.contains("user"))) {
              targetRow = rows[i];
              break;
            }
          }
          if (!targetRow) {
            for (let i = rows.length - 1; i >= 0; i--) {
              const raw = rows[i].getAttribute("data-step-index");
              const si = raw !== null && raw !== "" && !Number.isNaN(Number(raw)) ? Number(raw) : null;
              if (si !== null && si <= card.forkStepIndex) {
                targetRow = rows[i];
                break;
              }
            }
          }
        }
        if (!targetRow && card.forkStepIndex === undefined && rows.length > 0) {
          targetRow = rows[rows.length - 1];
        }
        if (targetRow && targetRow.id) {
          if (!cardsByRowId.has(targetRow.id)) cardsByRowId.set(targetRow.id, []);
          cardsByRowId.get(targetRow.id).push(card);
        }
      }
    }

    for (const row of rows) {
      const cards = cardsByRowId.get(row.id) || [];
      let container = row.querySelector(":scope > .btw-inline-cards");
      if (cards.length === 0) {
        if (container) container.remove();
        continue;
      }
      const sig = cards.map((c) => `${c.cascadeId}|${c.status || ""}|${c.title || ""}|${c.answerPreview || ""}`).join("||");
      if (container && container.getAttribute("data-sig") === sig) continue;
      if (!container) {
        container = document.createElement("div");
        container.className = "btw-inline-cards";
        row.appendChild(container);
      }
      container.setAttribute("data-sig", sig);
      container.innerHTML = cards
        .map((c) => {
          const idEsc = escapeHtmlLocal(c.cascadeId || "");
          const titleEsc = escapeHtmlLocal(c.title || c.question || "旁路快问");
          const isRunning = c.status === "CASCADE_RUN_STATUS_RUNNING";
          const isError = c.status === "CASCADE_RUN_STATUS_ERROR";
          const dotClass = isRunning ? "running" : isError ? "error" : "done";
          const preview = String(c.answerPreview || "").trim();
          const previewHtml = preview
            ? `<div class="btw-inline-card-preview">${escapeHtmlLocal(preview)}</div>`
            : isRunning
            ? `<div class="btw-inline-card-preview">正在独立生成回复…</div>`
            : "";
          return `
            <button type="button" class="btw-inline-card" data-btw-branch-id="${idEsc}">
              <div class="btw-inline-card-head">
                <span class="btw-inline-card-tag">⑂ 旁路快问</span>
                <span class="btw-inline-card-title">${titleEsc}</span>
                <span class="btw-inline-card-dot ${dotClass}"></span>
                <span class="btw-inline-card-arrow">›</span>
              </div>
              ${previewHtml}
            </button>
          `;
        })
        .join("");
      container.querySelectorAll("[data-btw-branch-id]").forEach((btn) => {
        btn.addEventListener("click", (e) => {
          e.stopPropagation();
          const targetId = btn.getAttribute("data-btw-branch-id");
          if (targetId && typeof navigateTo === "function") {
            navigateTo("#c=" + targetId);
          }
        });
      });
    }
  }

  function renderBtwQuote() {
    const card = document.getElementById("btw-quote-card");
    const txt = document.getElementById("btw-quote-text");
    const lbl = document.getElementById("btw-quote-label");
    if (!card || !txt) return;
    if (activeBtwStepIndex !== null) {
      if (lbl) lbl.textContent = "⑂ 承接至所选消息的上下文";
      txt.textContent = activeBtwPreview || `将从 Step #${activeBtwStepIndex} 处分叉，不携带后续消息。`;
      card.classList.remove("hidden");
    } else if (activeBtwQuote) {
      if (lbl) lbl.textContent = "引用片段";
      txt.textContent = activeBtwQuote;
      card.classList.remove("hidden");
    } else {
      txt.textContent = "";
      card.classList.add("hidden");
    }
  }

  async function refreshBtwHistory(parentId) {
    const bar = document.getElementById("btw-history-bar");
    if (!bar || !parentId) {
      bar?.classList.add("hidden");
      return;
    }
    try {
      const res = await fetch(`/gateway/btw/list?cascadeId=${encodeURIComponent(parentId)}`);
      if (!res.ok) return;
      const data = await res.json();
      const items = Array.isArray(data.items) ? data.items : [];
      if (!items.length) {
        bar.classList.add("hidden");
        bar.innerHTML = "";
        return;
      }
      bar.classList.remove("hidden");
      bar.innerHTML =
        `<div class="btw-branch-list-title">已有旁路分支 (${items.length})</div>` +
        items
          .map((it) => {
            const cid = escapeHtmlLocal(it.cascadeId || "");
            const title = escapeHtmlLocal(it.title || it.question || cid.slice(0, 8));
            const preview = String(it.answerPreview || "").trim();
            const isRunning = it.status === "CASCADE_RUN_STATUS_RUNNING";
            return `
              <button type="button" class="btw-branch-list-item" data-btw-open-id="${cid}">
                <span class="btw-origin-icon">⑂</span>
                <div class="btw-branch-list-item-text">
                  <div class="btw-branch-list-item-title">${title}</div>
                  ${preview ? `<div class="btw-branch-list-item-sub">${escapeHtmlLocal(preview)}</div>` : ""}
                </div>
                ${isRunning ? `<span class="btw-inline-card-dot running"></span>` : ""}
                <span class="btw-inline-card-arrow">›</span>
              </button>
            `;
          })
          .join("");
      bar.querySelectorAll("[data-btw-open-id]").forEach((btn) => {
        btn.addEventListener("click", () => {
          const cid = btn.getAttribute("data-btw-open-id");
          if (!cid) return;
          document.getElementById("dialog-btw")?.close();
          if (typeof navigateTo === "function") navigateTo("#c=" + cid);
        });
      });
    } catch (_) {}
  }

  async function startBtwAndOpen({ parentCascadeId, question, stepIndex = null, quote = "" } = {}) {
    const pid = parentCascadeId || (typeof activeCascadeId !== "undefined" ? activeCascadeId : "");
    const q = String(question || "").trim();
    if (!q || !pid) return false;
    const sendBtn = document.getElementById("btn-btw-send");
    const input = document.getElementById("btw-question-input");
    if (sendBtn) sendBtn.disabled = true;
    try {
      const payload = {
        parentCascadeId: pid,
        question: q,
        model: typeof activeModel !== "undefined" ? activeModel : "",
      };
      if (stepIndex !== null && stepIndex !== undefined && !Number.isNaN(Number(stepIndex)) && Number(stepIndex) >= 0) {
        payload.stepIndex = Number(stepIndex);
      } else if (quote && String(quote).trim()) {
        payload.quote = String(quote).trim();
      }
      const res = await fetch("/gateway/btw/start", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok || !data.cascadeId) throw new Error(data.error || "发起 /btw 失败");
      if (input) input.value = "";
      document.getElementById("dialog-btw")?.close();
      if (typeof loadConversations === "function") loadConversations();
      if (typeof navigateTo === "function") navigateTo("#c=" + data.cascadeId);
      return true;
    } catch (err) {
      window.showToast?.(err.message || "发起 /btw 失败", 2800);
      return false;
    } finally {
      if (sendBtn) sendBtn.disabled = false;
    }
  }

  function openBtwModal({ parentCascadeId, stepIndex = null, preview = "", quote = "", initialQuestion = "" } = {}) {
    const dlg = document.getElementById("dialog-btw");
    if (!dlg) return;
    const parentId = parentCascadeId || (typeof activeCascadeId !== "undefined" ? activeCascadeId : "");
    if (!parentId) {
      window.showToast?.("请先进入一个会话后再使用 /btw");
      return;
    }
    if (initialQuestion && initialQuestion.trim()) {
      startBtwAndOpen({ parentCascadeId: parentId, question: initialQuestion.trim(), stepIndex, quote });
      return;
    }
    activeBtwParentId = parentId;
    activeBtwStepIndex = stepIndex !== null && stepIndex !== undefined && !Number.isNaN(Number(stepIndex)) ? Number(stepIndex) : null;
    activeBtwPreview = String(preview || "").trim().slice(0, 220);
    activeBtwQuote = activeBtwStepIndex === null ? String(quote || "").trim() : "";

    renderBtwQuote();
    refreshBtwHistory(parentId);

    const input = document.getElementById("btw-question-input");
    if (input) input.value = "";
    if (typeof dlg.showModal === "function" && !dlg.open) dlg.showModal();
    setTimeout(() => input?.focus(), 60);
  }

  function initBtwDialog() {
    const dlg = document.getElementById("dialog-btw");
    document.getElementById("btn-btw-dialog-close")?.addEventListener("click", () => {
      dlg?.close();
    });
    document.getElementById("btn-btw-clear-quote")?.addEventListener("click", () => {
      activeBtwStepIndex = null;
      activeBtwPreview = "";
      activeBtwQuote = "";
      renderBtwQuote();
    });
    document.getElementById("btn-btw-send")?.addEventListener("click", () => {
      const q = document.getElementById("btw-question-input")?.value || "";
      startBtwAndOpen({
        parentCascadeId: activeBtwParentId,
        question: q,
        stepIndex: activeBtwStepIndex,
        quote: activeBtwQuote,
      });
    });
    document.getElementById("btw-question-input")?.addEventListener("keydown", (e) => {
      if (e.isComposing || e.keyCode === 229) return;
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        startBtwAndOpen({
          parentCascadeId: activeBtwParentId,
          question: e.target.value,
          stepIndex: activeBtwStepIndex,
          quote: activeBtwQuote,
        });
      }
    });
  }

  // --- 6. In-App Find in Page (TreeWalker + CSS.highlights + <dialog popover="manual">) ---
  function clearFindHighlights() {
    if ("highlights" in CSS) {
      // MANDATORY: Clear previous highlights before registering new ones.
      CSS.highlights.clear();
    }
    if (fallbackMarks.length > 0) {
      for (const mark of fallbackMarks) {
        const parent = mark.parentNode;
        if (parent) {
          parent.replaceChild(document.createTextNode(mark.textContent || ""), mark);
          parent.normalize();
        }
      }
      fallbackMarks = [];
    }
    currentFindMatches = [];
    currentFindIndex = -1;
  }

  function collectTextNodes(rootEl) {
    // MANDATORY: Use TreeWalker to collect text nodes — do not manipulate innerHTML.
    const walker = document.createTreeWalker(rootEl, NodeFilter.SHOW_TEXT);
    const nodes = [];
    let node;
    while ((node = walker.nextNode())) {
      if (node.nodeValue && node.nodeValue.length > 0) nodes.push(node);
    }
    return nodes;
  }

  function applyFindHighlights() {
    if ("highlights" in CSS) {
      // MANDATORY: Clear previous highlights before registering new ones.
      CSS.highlights.clear();
      if (currentFindMatches.length > 0) {
        const resultsHighlight = new Highlight();
        for (let i = 0; i < currentFindMatches.length; i++) {
          resultsHighlight.add(currentFindMatches[i]);
        }
        CSS.highlights.set("find-results", resultsHighlight);
        if (currentFindIndex >= 0 && currentFindMatches[currentFindIndex]) {
          const currentHighlight = new Highlight();
          currentHighlight.add(currentFindMatches[currentFindIndex]);
          CSS.highlights.set("find-current", currentHighlight);
        }
      }
    } else if (fallbackMarks.length > 0) {
      fallbackMarks.forEach((m, idx) => m.classList.toggle("active", idx === currentFindIndex));
    }

    const countEl = document.getElementById("md-find-count");
    if (countEl) {
      countEl.textContent = currentFindMatches.length > 0 ? `${currentFindIndex + 1}/${currentFindMatches.length}` : "0/0";
    }
    if (currentFindIndex >= 0) {
      const target = currentFindMatches[currentFindIndex];
      const anchorEl = target?.startContainer?.parentElement || fallbackMarks[currentFindIndex];
      anchorEl?.scrollIntoView?.({ block: "center", behavior: "smooth" });
    }
  }

  function runFindQuery(query) {
    clearFindHighlights();
    const rootEl = currentFindRoot || document.getElementById("md-viewer-content");
    const q = String(query || "").trim().toLowerCase();
    if (!rootEl || !q) {
      const countEl = document.getElementById("md-find-count");
      if (countEl) countEl.textContent = "0/0";
      return;
    }

    const nodes = collectTextNodes(rootEl);
    for (const node of nodes) {
      const lower = node.nodeValue.toLowerCase();
      let pos = 0;
      while ((pos = lower.indexOf(q, pos)) !== -1) {
        const range = document.createRange();
        range.setStart(node, pos);
        range.setEnd(node, pos + q.length);
        currentFindMatches.push(range);
        pos += q.length;
      }
    }

    if (!("highlights" in CSS) && currentFindMatches.length > 0) {
      // Old browser fallback using Range.surroundContents backwards so offsets stay valid
      for (let i = currentFindMatches.length - 1; i >= 0; i--) {
        try {
          const mark = document.createElement("mark");
          mark.className = "md-find-mark";
          currentFindMatches[i].surroundContents(mark);
          fallbackMarks.unshift(mark);
        } catch (_) {}
      }
      currentFindMatches = fallbackMarks;
    }

    if (currentFindMatches.length > 0) {
      currentFindIndex = 0;
    }
    applyFindHighlights();
  }

  function stepFindMatch(delta) {
    if (!currentFindMatches.length) return;
    currentFindIndex = (currentFindIndex + delta + currentFindMatches.length) % currentFindMatches.length;
    applyFindHighlights();
  }

  function openFindPopover(rootEl) {
    currentFindRoot = rootEl || document.getElementById("md-viewer-content");
    const pop = document.getElementById("md-find-dialog");
    const input = document.getElementById("md-find-input");
    if (!pop) return;
    if (typeof pop.showPopover === "function") {
      try {
        if (!pop.matches(":popover-open")) pop.showPopover();
      } catch (_) {
        pop.classList.add("open");
      }
    } else {
      pop.classList.add("open");
    }
    setTimeout(() => input?.focus(), 40);
  }

  let findInputDebounceTimer = 0;

  function closeFindPopover() {
    clearTimeout(findInputDebounceTimer);
    const pop = document.getElementById("md-find-dialog");
    if (pop) {
      if (typeof pop.hidePopover === "function") {
        try {
          if (pop.matches(":popover-open")) pop.hidePopover();
        } catch (_) {}
      }
      pop.classList.remove("open");
    }
    clearFindHighlights();
  }

  function initFindInPage() {
    document.getElementById("btn-md-viewer-find")?.addEventListener("click", () => {
      openFindPopover(document.getElementById("md-viewer-content"));
    });
    document.getElementById("btn-md-viewer-close")?.addEventListener("click", () => {
      closeFindPopover();
      window.closeMarkdownViewer?.();
    });
    document.getElementById("btn-md-find-close")?.addEventListener("click", closeFindPopover);
    document.getElementById("btn-md-find-prev")?.addEventListener("click", () => stepFindMatch(-1));
    document.getElementById("btn-md-find-next")?.addEventListener("click", () => stepFindMatch(1));
    document.getElementById("md-find-input")?.addEventListener("input", (e) => {
      const val = e.target.value;
      clearTimeout(findInputDebounceTimer);
      findInputDebounceTimer = setTimeout(() => runFindQuery(val), 150);
    });
    document.getElementById("md-find-input")?.addEventListener("keydown", (e) => {
      if (e.key === "Enter") {
        e.preventDefault();
        if (findInputDebounceTimer) {
          clearTimeout(findInputDebounceTimer);
          findInputDebounceTimer = 0;
          runFindQuery(e.target.value);
          return;
        }
        stepFindMatch(e.shiftKey ? -1 : 1);
      } else if (e.key === "Escape") {
        closeFindPopover();
      }
    });
  }

  // --- 7. 200 MB Audio/Video/.amr & PDF/Code Preview Modal ---
  function openMediaPreviewModal(uri, title) {
    const dlg = document.getElementById("dialog-media-preview");
    const bodyEl = document.getElementById("media-dialog-body");
    const titleEl = document.getElementById("media-dialog-title");
    const extLink = document.getElementById("media-dialog-open-ext");
    const findBtn = document.getElementById("btn-media-dialog-find");
    if (!dlg || !bodyEl) return false;

    const ext = fileExt(uri || title);
    const rawUrl = typeof window.resolveMediaRawUrl === "function" ? window.resolveMediaRawUrl(uri) : `/gateway/files/raw?uri=${encodeURIComponent(uri)}`;

    if (titleEl) titleEl.textContent = title || (uri.split("/").pop() || "文件预览");
    if (extLink) extLink.href = rawUrl;
    findBtn?.classList.add("hidden");

    if (AUDIO_EXTS.has(ext)) {
      const amrHint = ext === "amr"
        ? `<div style="font-size:12px;color:var(--ios-secondary-label);margin-top:8px;">若当前浏览器内核不支持直接解码 .amr，请点下方链接下载播放；网关已完整支持 200 MB 音频上传与识别。</div>`
        : "";
      bodyEl.innerHTML = `<audio class="v233-audio-player" controls autoplay src="${escapeHtmlLocal(rawUrl)}"></audio>${amrHint}`;
    } else if (VIDEO_EXTS.has(ext)) {
      bodyEl.innerHTML = `<video controls playsinline style="width:100%;max-height:58dvh;border-radius:10px;background:#000;" src="${escapeHtmlLocal(rawUrl)}"></video>`;
    } else if (ext === "pdf") {
      bodyEl.innerHTML = `<iframe loading="lazy" title="PDF Preview" src="${escapeHtmlLocal(rawUrl)}" style="width:100%;height:62dvh;border:none;border-radius:10px;background:#fff;"></iframe>`;
    } else {
      return false;
    }

    if (typeof dlg.showModal === "function") dlg.showModal();
    return true;
  }

  function initMediaDialog() {
    const dlg = document.getElementById("dialog-media-preview");
    document.getElementById("btn-media-dialog-close")?.addEventListener("click", () => {
      const body = document.getElementById("media-dialog-body");
      if (body) body.innerHTML = "";
      dlg?.close();
    });
    dlg?.addEventListener("close", () => {
      const body = document.getElementById("media-dialog-body");
      if (body) body.innerHTML = "";
    });
    document.getElementById("messages-stream")?.addEventListener("click", (e) => {
      const attCard = e.target.closest('[data-action="open-attachment-file"]');
      if (attCard) {
        const uri = attCard.getAttribute("data-uri") || "";
        const name = attCard.getAttribute("data-name") || "";
        const ext = fileExt(name || uri);
        if (AUDIO_EXTS.has(ext) || VIDEO_EXTS.has(ext) || ext === "pdf") {
          e.preventDefault();
          e.stopPropagation();
          openMediaPreviewModal(uri, name || uri);
          return;
        }
        if (ext === "r" || ext === "rmd" || ext === "rmarkdown" || ext === "rprofile") {
          e.preventDefault();
          e.stopPropagation();
          window.openMarkdownViewer?.(uri, name || uri);
          return;
        }
      }
      const prBtn = e.target.closest(".v233-pr-pill");
      if (prBtn) {
        const u = decodeURIComponent(prBtn.getAttribute("data-pr-url") || "");
        const pr = prCache.get(u);
        if (pr) openPRDetailModal(pr);
        return;
      }
      const link = e.target.closest("a[href]");
      if (link && !link.classList.contains("markdown-file-link")) {
        const href = link.getAttribute("href") || "";
        const ext = fileExt(href);
        if (AUDIO_EXTS.has(ext) || VIDEO_EXTS.has(ext) || ext === "pdf") {
          e.preventDefault();
          openMediaPreviewModal(href, link.textContent.trim());
        }
      }
    }, true);
    document.getElementById("btn-pr-dialog-close")?.addEventListener("click", () => {
      document.getElementById("dialog-github-pr")?.close();
    });
  }

  function init() {
    initThemeSettings();
    installRHighlighter();
    initTrustBanner();
    initBtwDialog();
    initFindInPage();
    initMediaDialog();
  }

  window.V233 = {
    AUDIO_EXTS,
    VIDEO_EXTS,
    R_LANGS,
    applyTheme,
    openSettingsSubpage,
    isAudioOrVideoFile,
    maxUploadBytesForFile,
    hydrateChatPRPills,
    checkWorkspaceTrust,
    openBtwModal,
    startBtwAndOpen,
    syncBranchState,
    applyBranchDecorations,
    openFindPopover,
    closeFindPopover,
    openMediaPreviewModal,
  };

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
