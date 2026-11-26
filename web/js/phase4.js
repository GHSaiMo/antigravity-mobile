// Phase 4 Native Feature Pack for Web & PWA (Antigravity 2.35.2)
// Covers: BrowserViewerService live frame & touch takeover, RemotingService SSH + interactive prompts,
// Marketplace search/featured, Plugin uninstall & OAuth hybrid closure (host browser + loopback forwarder),
// Desktop self-update & changelog, Workspace Code/File search, Artifact snapshots, Worktree PR, Conversation Starring.

(function () {
  const _t = (s) => (typeof t === "function" ? t(s) : s);
  const _esc = (s) => (typeof escapeHtml === "function" ? escapeHtml(s) : String(s || "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c])));

  const Phase4 = {
    _bv: {
      open: false,
      sessionId: "",
      tabId: "",
      controlToken: "",
      tabs: [],
      widthPx: 1280,
      heightPx: 800,
      pollTimer: 0,
      renewTick: 0,
    },
    _sshPollTimer: 0,
    _activeSnapshots: [],
    _activeViewerOrig: null,

    init() {
      this.bindDesktopUpdateRow();
      this.bindArtifactHistoryButton();
    },

    // --- 1. BrowserViewerService (Live Frame Polling + Click/Scroll/Paste Takeover) ---
    ensureBrowserOverlay() {
      let el = document.getElementById("p4-browser-overlay");
      if (el) return el;
      el = document.createElement("div");
      el.id = "p4-browser-overlay";
      el.className = "p4-browser-overlay hidden";
      el.innerHTML = `
        <div class="p4-bv-topbar">
          <button type="button" class="p4-bv-icon" id="p4-bv-close" title="关闭">✕</button>
          <button type="button" class="p4-bv-icon" id="p4-bv-back" title="后退">‹</button>
          <button type="button" class="p4-bv-icon" id="p4-bv-fwd" title="前进">›</button>
          <button type="button" class="p4-bv-icon" id="p4-bv-reload" title="刷新">↻</button>
          <input type="url" id="p4-bv-url" class="p4-bv-url" placeholder="https://..." />
          <button type="button" class="p4-bv-go" id="p4-bv-go">跳转</button>
          <select id="p4-bv-tabs" class="p4-bv-tabs hidden" aria-label="切换标签页"></select>
        </div>
        <div id="p4-bv-notice" class="p4-bv-notice hidden"></div>
        <div class="p4-bv-stage" id="p4-bv-stage">
          <div id="p4-bv-loading" class="p4-bv-loading"><div class="ios-spinner"></div><p>正在连接宿主浏览器画面…</p></div>
          <img id="p4-bv-img" class="p4-bv-img hidden" alt="宿主浏览器实时画面" draggable="false" />
        </div>
        <div class="p4-bv-controls">
          <div class="p4-bv-actions">
            <button type="button" class="p3-mini-btn primary" id="p4-bv-takeover">点击接管</button>
            <button type="button" class="p3-mini-btn" id="p4-bv-up">↑ 上滚</button>
            <button type="button" class="p3-mini-btn" id="p4-bv-down">↓ 下滚</button>
            <button type="button" class="p3-mini-btn" data-bv-key="Enter">Enter</button>
            <button type="button" class="p3-mini-btn" data-bv-key="Backspace">⌫</button>
            <button type="button" class="p3-mini-btn" data-bv-key="Escape">Esc</button>
          </div>
          <div class="p4-bv-paste-row">
            <input type="text" id="p4-bv-text" class="p3-input" placeholder="输入并粘贴文本到页面焦点…" />
            <button type="button" class="p3-mini-btn primary" id="p4-bv-send-text">发送</button>
          </div>
        </div>`;
      document.body.appendChild(el);

      document.getElementById("p4-bv-close")?.addEventListener("click", () => this.closeBrowserViewer());
      document.getElementById("p4-bv-back")?.addEventListener("click", () => this.sendBrowserAction("BROWSER_VIEWER_CONTROL_ACTION_BACK"));
      document.getElementById("p4-bv-fwd")?.addEventListener("click", () => this.sendBrowserAction("BROWSER_VIEWER_CONTROL_ACTION_FORWARD"));
      document.getElementById("p4-bv-reload")?.addEventListener("click", () => this.sendBrowserAction("BROWSER_VIEWER_CONTROL_ACTION_RELOAD"));
      document.getElementById("p4-bv-go")?.addEventListener("click", () => {
        const raw = document.getElementById("p4-bv-url")?.value.trim();
        if (!raw) return;
        const url = raw.includes("://") ? raw : "https://" + raw;
        document.getElementById("p4-bv-url").value = url;
        this.sendBrowserAction("BROWSER_VIEWER_CONTROL_ACTION_NAVIGATE", { url });
      });
      document.getElementById("p4-bv-url")?.addEventListener("keydown", (e) => {
        if (e.key === "Enter") document.getElementById("p4-bv-go")?.click();
      });
      document.getElementById("p4-bv-tabs")?.addEventListener("change", (e) => {
        const tid = e.target.value;
        if (tid) this.openBrowserTab(tid);
      });
      document.getElementById("p4-bv-takeover")?.addEventListener("click", async () => {
        if (this._bv.controlToken) {
          await this.releaseBrowserControl();
        } else {
          await this.ensureBrowserControl();
        }
      });
      document.getElementById("p4-bv-up")?.addEventListener("click", () => {
        this.sendBrowserAction("BROWSER_VIEWER_CONTROL_ACTION_INPUT_EVENT", {
          inputEvent: { kind: "KIND_WHEEL", xPx: Math.round(this._bv.widthPx / 2), yPx: Math.round(this._bv.heightPx / 2), deltaY: -420 },
        });
      });
      document.getElementById("p4-bv-down")?.addEventListener("click", () => {
        this.sendBrowserAction("BROWSER_VIEWER_CONTROL_ACTION_INPUT_EVENT", {
          inputEvent: { kind: "KIND_WHEEL", xPx: Math.round(this._bv.widthPx / 2), yPx: Math.round(this._bv.heightPx / 2), deltaY: 420 },
        });
      });
      el.querySelectorAll("[data-bv-key]").forEach((btn) => {
        btn.addEventListener("click", async () => {
          const key = btn.getAttribute("data-bv-key");
          await this.sendBrowserAction("BROWSER_VIEWER_CONTROL_ACTION_INPUT_EVENT", { inputEvent: { kind: "KIND_KEY_DOWN", key } });
          await this.sendBrowserAction("BROWSER_VIEWER_CONTROL_ACTION_INPUT_EVENT", { inputEvent: { kind: "KIND_KEY_UP", key } });
        });
      });
      document.getElementById("p4-bv-send-text")?.addEventListener("click", async () => {
        const inp = document.getElementById("p4-bv-text");
        const text = inp?.value || "";
        if (!text) return;
        inp.value = "";
        await this.sendBrowserAction("BROWSER_VIEWER_CONTROL_ACTION_INPUT_EVENT", { inputEvent: { kind: "KIND_PASTE", text } });
      });
      document.getElementById("p4-bv-text")?.addEventListener("keydown", (e) => {
        if (e.key === "Enter") document.getElementById("p4-bv-send-text")?.click();
      });

      const img = document.getElementById("p4-bv-img");
      img?.addEventListener("click", async (e) => {
        const rect = img.getBoundingClientRect();
        if (!rect.width || !rect.height) return;
        const rx = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
        const ry = Math.max(0, Math.min(1, (e.clientY - rect.top) / rect.height));
        const xPx = Math.round(rx * this._bv.widthPx);
        const yPx = Math.round(ry * this._bv.heightPx);
        await this.sendBrowserAction("BROWSER_VIEWER_CONTROL_ACTION_INPUT_EVENT", {
          inputEvent: { kind: "KIND_POINTER_DOWN", xPx, yPx, button: "MOUSE_BUTTON_LEFT" },
        });
        await this.sendBrowserAction("BROWSER_VIEWER_CONTROL_ACTION_INPUT_EVENT", {
          inputEvent: { kind: "KIND_POINTER_UP", xPx, yPx, button: "MOUSE_BUTTON_LEFT" },
        });
      });
      return el;
    },

    setBrowserNotice(msg, isErr) {
      const el = document.getElementById("p4-bv-notice");
      if (!el) return;
      if (!msg) {
        el.classList.add("hidden");
        el.textContent = "";
        return;
      }
      el.textContent = msg;
      el.classList.toggle("err", !!isErr);
      el.classList.remove("hidden");
    },

    updateTakeoverButton() {
      const btn = document.getElementById("p4-bv-takeover");
      if (!btn) return;
      btn.textContent = this._bv.controlToken ? _t("释放接管") : _t("点击接管");
    },

    async openBrowserViewer({ initialUrl = "", initialTabId = "" } = {}) {
      const overlay = this.ensureBrowserOverlay();
      overlay.classList.remove("hidden");
      this._bv.open = true;
      this.setBrowserNotice("", false);
      await this.openBrowserTab(initialTabId, initialUrl);
    },

    async openBrowserTab(tabId, navigateUrl) {
      if (this._bv.pollTimer) {
        clearInterval(this._bv.pollTimer);
        this._bv.pollTimer = 0;
      }
      if (this._bv.sessionId && this._bv.tabId) {
        if (this._bv.controlToken) {
          fetch("/gateway/browser/release", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ sessionId: this._bv.sessionId, tabId: this._bv.tabId, controlToken: this._bv.controlToken }),
          }).catch(() => {});
        }
        fetch("/gateway/browser/close", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ sessionId: this._bv.sessionId, tabId: this._bv.tabId }),
        }).catch(() => {});
      }
      this._bv.sessionId = "";
      this._bv.tabId = "";
      this._bv.controlToken = "";
      this.updateTakeoverButton();

      const loading = document.getElementById("p4-bv-loading");
      const img = document.getElementById("p4-bv-img");
      loading?.classList.remove("hidden");
      img?.classList.add("hidden");

      try {
        const tRes = await fetch("/gateway/browser/tabs");
        const tData = await tRes.json().catch(() => ({}));
        this._bv.tabs = Array.isArray(tData.tabs) ? tData.tabs : [];
        const sel = document.getElementById("p4-bv-tabs");
        if (sel) {
          if (this._bv.tabs.length > 1) {
            sel.innerHTML = this._bv.tabs.map((tb) => `<option value="${_esc(tb.tabId)}">${_esc(tb.title || tb.url || tb.tabId)}</option>`).join("");
            sel.classList.remove("hidden");
          } else {
            sel.classList.add("hidden");
          }
        }
        const chosen = tabId || this._bv.tabs.find((x) => x.active)?.tabId || this._bv.tabs[0]?.tabId || "";
        const oRes = await fetch("/gateway/browser/open", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ tabId: chosen, leaseSeconds: 60 }),
        });
        const oData = await oRes.json().catch(() => ({}));
        if (!oRes.ok || !oData.sessionId) {
          this.setBrowserNotice(oData.error || _t("宿主浏览器未启动或无可用标签页"), true);
          loading?.classList.add("hidden");
          return;
        }
        this._bv.sessionId = oData.sessionId;
        this._bv.tabId = oData.tab?.tabId || chosen;
        if (sel && this._bv.tabId) sel.value = this._bv.tabId;
        const urlEl = document.getElementById("p4-bv-url");
        if (urlEl) urlEl.value = navigateUrl || oData.tab?.url || "";

        if (navigateUrl) {
          await this.sendBrowserAction("BROWSER_VIEWER_CONTROL_ACTION_NAVIGATE", { url: navigateUrl });
        } else {
          await this.pullBrowserFrame();
        }
        this._bv.renewTick = 0;
        this._bv.pollTimer = setInterval(() => {
          if (!this._bv.open) return;
          this.pullBrowserFrame();
          this._bv.renewTick++;
          if (this._bv.renewTick % 18 === 0 && this._bv.sessionId && this._bv.tabId) {
            fetch("/gateway/browser/renew", {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ sessionId: this._bv.sessionId, tabId: this._bv.tabId, leaseSeconds: 60 }),
            }).catch(() => {});
          }
        }, 850);
      } catch (err) {
        this.setBrowserNotice(err.message || _t("无法连接宿主浏览器"), true);
        loading?.classList.add("hidden");
      }
    },

    async pullBrowserFrame() {
      if (!this._bv.sessionId || !this._bv.tabId) return;
      try {
        const res = await fetch(`/gateway/browser/frame?sessionId=${encodeURIComponent(this._bv.sessionId)}&tabId=${encodeURIComponent(this._bv.tabId)}`);
        const d = await res.json().catch(() => ({}));
        if (d.widthPx > 0) this._bv.widthPx = d.widthPx;
        if (d.heightPx > 0) this._bv.heightPx = d.heightPx;
        if (d.imageData) {
          const src = d.imageData.startsWith("data:") ? d.imageData : `data:${d.mimeType || "image/jpeg"};base64,${d.imageData}`;
          const img = document.getElementById("p4-bv-img");
          const loading = document.getElementById("p4-bv-loading");
          if (img) {
            img.src = src;
            img.classList.remove("hidden");
          }
          loading?.classList.add("hidden");
        }
        if (d.tab?.url) {
          const urlEl = document.getElementById("p4-bv-url");
          if (urlEl && document.activeElement !== urlEl && !urlEl.value) urlEl.value = d.tab.url;
        }
      } catch (_) {}
    },

    async ensureBrowserControl() {
      if (this._bv.controlToken) return this._bv.controlToken;
      if (!this._bv.sessionId || !this._bv.tabId) return "";
      const res = await fetch("/gateway/browser/takeover", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ sessionId: this._bv.sessionId, tabId: this._bv.tabId, leaseSeconds: 90 }),
      });
      const d = await res.json().catch(() => ({}));
      if (d.controlToken) {
        this._bv.controlToken = d.controlToken;
        this.updateTakeoverButton();
        this.setBrowserNotice(_t("已接管宿主浏览器控制权"), false);
        return d.controlToken;
      }
      this.setBrowserNotice(d.error || _t("接管浏览器失败"), true);
      return "";
    },

    async releaseBrowserControl() {
      if (this._bv.sessionId && this._bv.tabId && this._bv.controlToken) {
        await fetch("/gateway/browser/release", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ sessionId: this._bv.sessionId, tabId: this._bv.tabId, controlToken: this._bv.controlToken }),
        }).catch(() => {});
      }
      this._bv.controlToken = "";
      this.updateTakeoverButton();
      this.setBrowserNotice(_t("已释放控制权"), false);
    },

    async sendBrowserAction(action, extra = {}) {
      const tok = await this.ensureBrowserControl();
      if (!tok) return;
      await fetch("/gateway/browser/control", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          sessionId: this._bv.sessionId,
          tabId: this._bv.tabId,
          controlToken: tok,
          action,
          ...extra,
        }),
      }).catch(() => {});
      setTimeout(() => this.pullBrowserFrame(), 120);
    },

    closeBrowserViewer() {
      this._bv.open = false;
      if (this._bv.pollTimer) {
        clearInterval(this._bv.pollTimer);
        this._bv.pollTimer = 0;
      }
      if (this._bv.sessionId && this._bv.tabId) {
        const sid = this._bv.sessionId;
        const tid = this._bv.tabId;
        const tok = this._bv.controlToken;
        if (tok) {
          fetch("/gateway/browser/release", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ sessionId: sid, tabId: tid, controlToken: tok }),
          }).catch(() => {});
        }
        fetch("/gateway/browser/close", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ sessionId: sid, tabId: tid }),
        }).catch(() => {});
      }
      this._bv.sessionId = "";
      this._bv.tabId = "";
      this._bv.controlToken = "";
      document.getElementById("p4-browser-overlay")?.classList.add("hidden");
    },

    // --- 2. OAuth Hybrid Modal (Host Browser Takeover + Loopback Callback Forward + MCP Auth Code) ---
    openOAuthHybrid({ title = "OAuth 登录", startResponse = {}, mcpServerName = "", onCompleted = null } = {}) {
      let overlay = document.getElementById("p4-oauth-overlay");
      if (!overlay) {
        overlay = document.createElement("div");
        overlay.id = "p4-oauth-overlay";
        overlay.className = "ios-sheet-overlay hidden";
        document.body.appendChild(overlay);
      }
      const authUrl = startResponse.authUrl || "";
      const isLoopback = !!startResponse.isLoopbackCallback;
      overlay.innerHTML = `
        <div class="ios-sheet-card" style="max-height:82vh;">
          <div class="sheet-grabber"></div>
          <div class="sheet-header">
            <div style="width:48px;"></div>
            <h2 class="sheet-title">${_esc(title)}</h2>
            <button type="button" class="sheet-header-btn primary" id="p4-oauth-done">${_esc(_t("完成"))}</button>
          </div>
          <div class="sheet-body" style="padding:14px 16px 24px;display:flex;flex-direction:column;gap:10px;">
            <p class="settings-note" style="margin:0;">${_esc(isLoopback
              ? _t("该认证使用电脑本机回调地址 (127.0.0.1 / localhost)。推荐直接在宿主浏览器接管完成登录，或登录后将回调链接粘贴回此处转发给电脑端。")
              : _t("请打开下方授权链接完成登录，完成后点击「完成」。"))}</p>
            ${authUrl && isLoopback ? `<button type="button" class="git-btn primary" id="p4-oauth-host-browser">${_esc(_t("在宿主浏览器直接接管登录"))}</button>` : ""}
            ${authUrl ? `<a class="git-btn" style="text-align:center;text-decoration:none;" href="${_esc(authUrl)}" target="_blank" rel="noopener noreferrer">${_esc(_t("在手机系统浏览器打开"))} ↗</a>` : ""}
            ${isLoopback ? `
              <div class="p3-section-header" style="margin-top:4px;">${_esc(_t("备用：粘贴回调 URL 转发至电脑"))}</div>
              <input type="url" id="p4-oauth-cb-url" class="p3-input monospaced" placeholder="http://127.0.0.1:.../callback?code=..." />
              <button type="button" class="p3-mini-btn primary" id="p4-oauth-forward">${_esc(_t("转发回调完成授权"))}</button>
            ` : ""}
            ${mcpServerName ? `
              <div class="p3-section-header" style="margin-top:4px;">${_esc(_t("或手动提交 MCP 授权码 (Code)"))}</div>
              <input type="text" id="p4-oauth-mcp-code" class="p3-input monospaced" placeholder="code" />
              <button type="button" class="p3-mini-btn" id="p4-oauth-mcp-submit">${_esc(_t("提交 MCP 授权码"))}</button>
            ` : ""}
            <div id="p4-oauth-status" class="settings-note" style="margin:0;"></div>
          </div>
        </div>`;
      overlay.classList.remove("hidden");

      const close = () => {
        overlay.classList.add("hidden");
        onCompleted?.();
      };
      overlay.onclick = (e) => { if (e.target === overlay) close(); };
      document.getElementById("p4-oauth-done")?.addEventListener("click", close);
      document.getElementById("p4-oauth-host-browser")?.addEventListener("click", () => {
        overlay.classList.add("hidden");
        this.openBrowserViewer({ initialUrl: authUrl });
      });
      document.getElementById("p4-oauth-forward")?.addEventListener("click", async () => {
        const cb = document.getElementById("p4-oauth-cb-url")?.value.trim();
        const stEl = document.getElementById("p4-oauth-status");
        if (!cb) return;
        const r = await fetch("/gateway/oauth/callback-forward", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ callbackUrl: cb }),
        });
        const d = await r.json().catch(() => ({}));
        if (stEl) stEl.textContent = d.ok ? (d.message || _t("回调已成功转发至电脑端")) : (d.error || _t("转发失败"));
        if (d.ok) onCompleted?.();
      });
      document.getElementById("p4-oauth-mcp-submit")?.addEventListener("click", async () => {
        const code = document.getElementById("p4-oauth-mcp-code")?.value.trim();
        const stEl = document.getElementById("p4-oauth-status");
        if (!code) return;
        const r = await fetch("/gateway/mcp/oauth/complete", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ serverName: mcpServerName, code }),
        });
        const d = await r.json().catch(() => ({}));
        if (stEl) stEl.textContent = r.ok && d.ok !== false ? _t("MCP 授权已提交") : (d.error || _t("提交失败"));
        if (r.ok && d.ok !== false) onCompleted?.();
      });
    },

    // --- 3. Desktop Self-Update & Changelog (Settings -> About) ---
    bindDesktopUpdateRow() {
      const btn = document.getElementById("btn-settings-desktop-update");
      if (!btn) return;
      btn.addEventListener("click", () => this.openDesktopUpdateModal());
      this.refreshSettingsDesktopUpdateRow();
    },

    async refreshSettingsDesktopUpdateRow() {
      const verEl = document.getElementById("settings-desktop-version");
      const badgeEl = document.getElementById("settings-desktop-update-badge");
      if (!verEl && !badgeEl) return;
      try {
        const res = await fetch("/gateway/desktop/update");
        if (!res.ok) return;
        const st = await res.json();
        if (verEl && st.currentVersion) verEl.textContent = "v" + st.currentVersion;
        if (badgeEl) badgeEl.classList.toggle("hidden", !st.updateAvailable);
      } catch (_) {}
    },

    async openDesktopUpdateModal() {
      let overlay = document.getElementById("p4-desktop-update-overlay");
      if (!overlay) {
        overlay = document.createElement("div");
        overlay.id = "p4-desktop-update-overlay";
        overlay.className = "ios-sheet-overlay hidden";
        document.body.appendChild(overlay);
      }
      overlay.innerHTML = `
        <div class="ios-sheet-card tall-sheet-card">
          <div class="sheet-grabber"></div>
          <div class="sheet-header">
            <div style="width:48px;"></div>
            <h2 class="sheet-title">${_esc(_t("Antigravity 桌面端更新"))}</h2>
            <button type="button" class="sheet-header-btn primary" id="p4-du-close">${_esc(_t("完成"))}</button>
          </div>
          <div id="p4-du-body" class="sheet-body" style="padding:14px 16px 24px;">
            <div class="sheet-state"><div class="ios-spinner"></div><p>${_esc(_t("正在检查桌面端版本与更新日志…"))}</p></div>
          </div>
        </div>`;
      overlay.classList.remove("hidden");
      overlay.onclick = (e) => { if (e.target === overlay) overlay.classList.add("hidden"); };
      document.getElementById("p4-du-close")?.addEventListener("click", () => overlay.classList.add("hidden"));

      const body = document.getElementById("p4-du-body");
      try {
        const [stRes, clRes] = await Promise.all([
          fetch("/gateway/desktop/update").then((r) => r.json()).catch(() => ({})),
          fetch("/gateway/desktop/changelog").then((r) => r.json()).catch(() => ({})),
        ]);
        const cur = stRes.currentVersion || "-";
        const latest = stRes.latestVersion || cur;
        const mdHtml = clRes.markdown && typeof renderMarkdown === "function" ? renderMarkdown(clRes.markdown) : `<p>${_esc(clRes.markdown || _t("电脑端 Antigravity 已是最新版本。"))}</p>`;
        body.innerHTML = `
          <div class="p3-row" style="margin-bottom:10px;">
            <div class="p3-row-main">
              <div class="p3-row-title">${stRes.updateAvailable ? `v${_esc(cur)} → v${_esc(latest)}` : `v${_esc(cur)}`}</div>
              <div class="p3-row-sub">${stRes.updateAvailable ? _esc(_t("检测到桌面端新版本")) : _esc(_t("已是最新版本"))}</div>
            </div>
            ${stRes.updateAvailable ? `<button type="button" class="p3-mini-btn primary" id="p4-du-apply">${_esc(_t("立即更新桌面端"))}</button>` : ""}
          </div>
          ${stRes.updateAvailable && stRes.runningCount > 0 ? `<div class="p4-warn-box">${_esc(`注意：电脑端当前有 ${stRes.runningCount} 个活跃会话运行中，应用更新会重启桌面客户端。`)}</div>` : ""}
          <div class="md-viewer-markdown">${mdHtml}</div>`;
        document.getElementById("p4-du-apply")?.addEventListener("click", async (e) => {
          if (!confirm(_t("确认重启并更新桌面端？更新期间 Antigravity 将短暂重启。"))) return;
          e.target.disabled = true;
          const r = await fetch("/gateway/desktop/update/apply", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ confirm: true }),
          });
          const d = await r.json().catch(() => ({}));
          showToast?.(d.ok ? (d.message || _t("已下发桌面端更新指令")) : (d.error || _t("触发更新失败")), 3000);
          if (d.ok) overlay.classList.add("hidden");
          else e.target.disabled = false;
        });
      } catch (err) {
        body.innerHTML = `<div class="sheet-state warn"><p>${_esc(err.message)}</p></div>`;
      }
    },

    // --- 4. Workspace Code & File Search (Embedded in #sheet-changes) ---
    renderWorkspaceSearchTab(container, cascadeId, mode) {
      if (!container) return;
      const isCode = mode === "code";
      container.innerHTML = `
        <div class="p4-search-bar">
          <input type="search" id="p4-ws-q" class="p3-input" placeholder="${_esc(isCode ? _t("输入代码关键字或正则…") : _t("输入文件名关键字…"))}" />
          <button type="button" id="p4-ws-go" class="p3-mini-btn primary">${_esc(_t("搜索"))}</button>
        </div>
        ${isCode ? `
          <div class="p4-search-opts">
            <label><input type="checkbox" id="p4-ws-regex" /> ${_esc(_t("正则 .*"))}</label>
            <label><input type="checkbox" id="p4-ws-case" /> ${_esc(_t("区分大小写 Aa"))}</label>
          </div>` : ""}
        <div id="p4-ws-results" class="p4-search-results"><div class="p3-empty">${_esc(isCode ? _t("输入关键词检索工作区代码") : _t("输入文件名快速定位工作区文件"))}</div></div>`;

      const run = async () => {
        const q = document.getElementById("p4-ws-q")?.value.trim();
        const resEl = document.getElementById("p4-ws-results");
        if (!q || !resEl) return;
        resEl.innerHTML = `<div class="sheet-state"><div class="ios-spinner"></div><p>${_esc(_t("正在检索工作区…"))}</p></div>`;
        try {
          const endpoint = isCode ? "/gateway/search/code" : "/gateway/search/files";
          const payload = isCode
            ? { cascadeId, query: q, isRegex: !!document.getElementById("p4-ws-regex")?.checked, caseSensitive: !!document.getElementById("p4-ws-case")?.checked, maxResults: 60 }
            : { cascadeId, query: q, maxResults: 60 };
          const r = await fetch(endpoint, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(payload) });
          const d = await r.json().catch(() => ({}));
          const hits = Array.isArray(d.results) ? d.results : [];
          if (!hits.length) {
            resEl.innerHTML = `<div class="p3-empty">${_esc(isCode ? _t("未找到匹配的代码片段") : _t("未找到匹配的文件"))}</div>`;
            return;
          }
          resEl.innerHTML = hits.map((h, i) => isCode ? `
            <button type="button" class="p4-hit-card" data-hit-idx="${i}">
              <div class="p4-hit-head"><span class="monospaced">${_esc(h.relativePath || h.path)}</span><span class="p4-hit-line">L${Number(h.lineNumber) || 1}</span></div>
              <div class="p4-hit-snippet monospaced">${_esc((h.snippet || "").trim())}</div>
            </button>` : `
            <button type="button" class="p4-hit-card" data-hit-idx="${i}">
              <div class="p4-hit-head"><span class="monospaced">${h.isDirectory ? "📁 " : "📄 "}${_esc(h.relativePath || h.path)}</span></div>
            </button>`).join("");
          resEl.querySelectorAll("[data-hit-idx]").forEach((btn) => {
            btn.addEventListener("click", () => {
              const hit = hits[Number(btn.getAttribute("data-hit-idx"))];
              if (!hit || hit.isDirectory) return;
              if (typeof openMarkdownViewer === "function") {
                openMarkdownViewer(hit.uri || hit.path, hit.relativePath || hit.path);
              }
            });
          });
        } catch (err) {
          resEl.innerHTML = `<div class="sheet-state warn"><p>${_esc(err.message)}</p></div>`;
        }
      };

      document.getElementById("p4-ws-go")?.addEventListener("click", run);
      document.getElementById("p4-ws-q")?.addEventListener("keydown", (e) => {
        if (e.key === "Enter") run();
      });
    },

    // --- 5. RemotingService SSH Hosts (Embedded inside #sheet-new) ---
    async renderRemoteSshSection(container) {
      if (!container) return;
      container.innerHTML = `
        <div class="p4-ssh-section">
          <div class="p3-section-header">
            <span>${_esc(_t("SSH 远端主机"))}</span>
            <button type="button" class="p3-mini-btn" id="p4-ssh-add-toggle">${_esc(_t("+ 添加主机"))}</button>
          </div>
          <div id="p4-ssh-add-form" class="p3-form-card hidden">
            <input type="text" id="p4-ssh-name" class="p3-input" placeholder="${_esc(_t("主机名称"))}" />
            <input type="text" id="p4-ssh-host" class="p3-input monospaced" placeholder="${_esc(_t("主机地址 (IP / 域名)"))}" />
            <div style="display:flex;gap:8px;">
              <input type="text" id="p4-ssh-user" class="p3-input" style="flex:1;" placeholder="${_esc(_t("用户名"))}" />
              <input type="number" id="p4-ssh-port" class="p3-input monospaced" style="width:82px;" value="22" placeholder="22" />
            </div>
            <div class="p3-form-actions">
              <button type="button" class="p3-mini-btn" id="p4-ssh-add-cancel">${_esc(_t("取消"))}</button>
              <button type="button" class="p3-mini-btn primary" id="p4-ssh-add-save">${_esc(_t("保存"))}</button>
            </div>
          </div>
          <div id="p4-ssh-list" class="p3-list"><div class="p3-empty">${_esc(_t("正在加载远端主机…"))}</div></div>
        </div>`;

      document.getElementById("p4-ssh-add-toggle")?.addEventListener("click", () => {
        document.getElementById("p4-ssh-add-form")?.classList.toggle("hidden");
      });
      document.getElementById("p4-ssh-add-cancel")?.addEventListener("click", () => {
        document.getElementById("p4-ssh-add-form")?.classList.add("hidden");
      });
      document.getElementById("p4-ssh-add-save")?.addEventListener("click", async () => {
        const host = document.getElementById("p4-ssh-host")?.value.trim();
        const name = document.getElementById("p4-ssh-name")?.value.trim() || host;
        const user = document.getElementById("p4-ssh-user")?.value.trim();
        const port = parseInt(document.getElementById("p4-ssh-port")?.value || "22", 10) || 22;
        if (!host) return;
        const r = await fetch("/gateway/remotes/save", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ name, host, user, port }),
        });
        if (r.ok) {
          document.getElementById("p4-ssh-add-form")?.classList.add("hidden");
          this.renderRemoteSshSection(container);
        } else {
          showToast?.(_t("保存远端主机失败"));
        }
      });

      const listEl = document.getElementById("p4-ssh-list");
      try {
        const res = await fetch("/gateway/remotes");
        const data = await res.json().catch(() => ({}));
        const remotes = Array.isArray(data.remotes) ? data.remotes : [];
        const connectedIds = new Set((Array.isArray(data.connected) ? data.connected : []).map((c) => c.remoteId || c.name));
        if (!remotes.length) {
          listEl.innerHTML = `<div class="p3-empty">${_esc(_t("暂未配置 SSH 远端主机"))}</div>`;
          return;
        }
        listEl.innerHTML = remotes.map((rm, idx) => {
          const id = rm.remoteId || rm.name || "";
          const hop = rm.remoteMultiHop?.hops?.[0];
          const summary = hop?.host ? `${hop.user ? hop.user + "@" : ""}${hop.host}${hop.port && hop.port !== 22 ? ":" + hop.port : ""}` : (rm.description || "");
          const isConn = connectedIds.has(id);
          return `<div class="p3-row">
            <div class="p3-row-main">
              <div class="p3-row-title">${_esc(rm.name || summary || id)} ${isConn ? `<span class="p3-badge" style="color:#16a34a;">${_esc(_t("已连接"))}</span>` : ""}</div>
              ${summary ? `<div class="p3-row-sub monospaced">${_esc(summary)}</div>` : ""}
            </div>
            <div class="p3-row-actions">
              ${isConn
                ? `<button type="button" class="p3-mini-btn danger" data-ssh-disc="${idx}">${_esc(_t("断开"))}</button>`
                : `<button type="button" class="p3-mini-btn primary" data-ssh-conn="${idx}">${_esc(_t("连接"))}</button>`}
            </div>
          </div>`;
        }).join("");

        listEl.querySelectorAll("[data-ssh-disc]").forEach((btn) => {
          btn.addEventListener("click", async () => {
            const rm = remotes[Number(btn.getAttribute("data-ssh-disc"))];
            if (!rm) return;
            await fetch("/gateway/remotes/disconnect", {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ remoteId: rm.remoteId || rm.name, name: rm.name }),
            });
            this.renderRemoteSshSection(container);
          });
        });
        listEl.querySelectorAll("[data-ssh-conn]").forEach((btn) => {
          btn.addEventListener("click", async () => {
            const rm = remotes[Number(btn.getAttribute("data-ssh-conn"))];
            if (!rm) return;
            const hop = rm.remoteMultiHop?.hops?.[0] || {};
            const r = await fetch("/gateway/remotes/connect", {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ name: rm.name || hop.host, remoteId: rm.remoteId, host: hop.host, user: hop.user, port: hop.port }),
            });
            const d = await r.json().catch(() => ({}));
            if (d.job?.jobId) {
              this.pollSshConnectJob(d.job.jobId, rm.name || hop.host, container);
            } else {
              showToast?.(d.error || _t("发起 SSH 连接失败"), 2600);
            }
          });
        });
      } catch (_) {
        listEl.innerHTML = `<div class="p3-empty">${_esc(_t("暂未配置 SSH 远端主机"))}</div>`;
      }
    },

    pollSshConnectJob(jobId, name, container) {
      if (this._sshPollTimer) clearInterval(this._sshPollTimer);
      let promptOpen = false;
      showToast?.(`${_t("正在连接 SSH")} ${name}…`);
      this._sshPollTimer = setInterval(async () => {
        try {
          const r = await fetch(`/gateway/remotes/connect/status?jobId=${encodeURIComponent(jobId)}`);
          const d = await r.json().catch(() => ({}));
          const job = d.job;
          if (!job) return;
          if (job.status === "prompt" && !promptOpen) {
            promptOpen = true;
            const ans = prompt(job.prompt?.message || _t("请输入 SSH 认证信息 / 密码："));
            if (ans !== null) {
              await fetch("/gateway/remotes/prompt", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ jobId, response: ans }),
              });
            }
            promptOpen = false;
          } else if (job.status === "connected") {
            clearInterval(this._sshPollTimer);
            this._sshPollTimer = 0;
            showToast?.(`${_t("已连接远端主机")} ${name}`);
            this.renderRemoteSshSection(container);
            if (typeof openNewSheet === "function") openNewSheet();
          } else if (job.status === "error" || job.done) {
            clearInterval(this._sshPollTimer);
            this._sshPollTimer = 0;
            if (job.error) showToast?.(job.error, 3000);
          }
        } catch (_) {}
      }, 950);
    },

    // --- 6. Worktree PR Create / Update (Embedded in #sheet-git) ---
    async openWorktreePrDialog(cascadeId, onDone) {
      const title = prompt(_t("输入 Worktree PR 标题（留空由桌面端自动生成）："), "");
      if (title === null) return;
      const draft = confirm(_t("是否创建为草稿 PR (Draft)？（点「取消」则创建正式 PR）"));
      try {
        const r = await fetch("/gateway/worktree/pr", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ cascadeId, title: title.trim(), draft }),
        });
        const d = await r.json().catch(() => ({}));
        if (!r.ok || d.ok === false) throw new Error(d.error || `HTTP ${r.status}`);
        showToast?.(d.prUrl ? `PR 已同步: ${d.prUrl}` : _t("Worktree PR 已创建 / 更新"), 3200);
        if (d.prUrl && typeof isSafeURL === "function" && isSafeURL(d.prUrl)) {
          window.open(d.prUrl, "_blank", "noopener,noreferrer");
        }
        onDone?.();
      } catch (err) {
        showToast?.(_t("同步 PR 失败: ") + (err.message || err), 3000);
      }
    },

    // --- 7. Artifact Version Snapshots (Shown ONLY when snapshots.length > 1) ---
    bindArtifactHistoryButton() {
      document.getElementById("btn-md-viewer-history")?.addEventListener("click", () => {
        this.openArtifactSnapshotsPicker();
      });
    },

    async refreshArtifactSnapshots(cascadeId, uri) {
      const btn = document.getElementById("btn-md-viewer-history");
      this._activeSnapshots = [];
      if (btn) btn.classList.add("hidden");
      if (!cascadeId || !uri) return;
      try {
        const res = await fetch(`/gateway/artifacts/snapshots?cascadeId=${encodeURIComponent(cascadeId)}&artifactUri=${encodeURIComponent(uri)}`);
        const d = await res.json().catch(() => ({}));
        const list = Array.isArray(d.snapshots) ? d.snapshots : [];
        this._activeSnapshots = list;
        if (btn && list.length > 1) {
          btn.classList.remove("hidden");
          btn.title = `${_t("历史版本快照")} (${list.length})`;
        }
      } catch (_) {}
    },

    openArtifactSnapshotsPicker() {
      const snaps = this._activeSnapshots || [];
      if (snaps.length <= 1) return;
      let overlay = document.getElementById("p4-snapshots-overlay");
      if (!overlay) {
        overlay = document.createElement("div");
        overlay.id = "p4-snapshots-overlay";
        overlay.className = "ios-sheet-overlay hidden";
        document.body.appendChild(overlay);
      }
      overlay.innerHTML = `
        <div class="ios-sheet-card" style="max-height:76vh;">
          <div class="sheet-grabber"></div>
          <div class="sheet-header">
            <div style="width:48px;"></div>
            <h2 class="sheet-title">${_esc(_t("历史版本快照"))} (${snaps.length})</h2>
            <button type="button" class="sheet-header-btn primary" id="p4-snap-close">${_esc(_t("完成"))}</button>
          </div>
          <div class="sheet-body" style="padding:12px 16px 24px;display:flex;flex-direction:column;gap:8px;">
            ${snaps.map((s, idx) => `
              <button type="button" class="p4-hit-card" data-snap-idx="${idx}">
                <div class="p4-hit-head"><span>v${idx + 1} · ${_esc(s.artifactName || "")}</span><span class="p4-hit-line">${_esc(s.lastEdited || "")}</span></div>
                ${s.summary ? `<div class="p4-hit-snippet">${_esc(s.summary)}</div>` : ""}
              </button>`).join("")}
          </div>
        </div>`;
      overlay.classList.remove("hidden");
      overlay.onclick = (e) => { if (e.target === overlay) overlay.classList.add("hidden"); };
      document.getElementById("p4-snap-close")?.addEventListener("click", () => overlay.classList.add("hidden"));
      overlay.querySelectorAll("[data-snap-idx]").forEach((btn) => {
        btn.addEventListener("click", () => {
          const idx = Number(btn.getAttribute("data-snap-idx"));
          const snap = snaps[idx];
          overlay.classList.add("hidden");
          const contentEl = document.getElementById("md-viewer-content");
          if (snap && contentEl && typeof renderMarkdown === "function") {
            contentEl.innerHTML = `<div class="p4-warn-box" style="display:flex;justify-content:space-between;align-items:center;"><span>${_esc(`正在查看历史快照 v${idx + 1}`)}</span><button type="button" class="p3-mini-btn" id="p4-snap-latest">${_esc(_t("返回最新版"))}</button></div>` + renderMarkdown(snap.content || "");
            document.getElementById("p4-snap-latest")?.addEventListener("click", () => {
              if (typeof openMarkdownViewer === "function" && typeof currentViewerUri === "string") {
                openMarkdownViewer(currentViewerUri);
              }
            });
          }
        });
      });
    },

    // --- 8. Marketplace Search / Featured (Embedded in Tools -> Plugins Tab) ---
    async renderMarketplaceSection(container, installedIds, onInstalled) {
      if (!container) return;
      container.innerHTML = `
        <div class="p3-section-header" style="margin-top:14px;"><span>${_esc(_t("插件市场"))}</span></div>
        <div class="p4-search-bar" style="margin-bottom:8px;">
          <input type="search" id="p4-mkt-q" class="p3-input" placeholder="${_esc(_t("搜索插件市场…"))}" />
          <button type="button" id="p4-mkt-go" class="p3-mini-btn primary">${_esc(_t("搜索"))}</button>
        </div>
        <div id="p4-mkt-list" class="p3-list"><div class="p3-empty">${_esc(_t("加载精选插件中…"))}</div></div>`;

      const load = async (q) => {
        const listEl = document.getElementById("p4-mkt-list");
        if (!listEl) return;
        const url = q ? `/gateway/marketplace/search?q=${encodeURIComponent(q)}&limit=20` : "/gateway/marketplace/featured";
        try {
          const r = await fetch(url);
          const d = await r.json().catch(() => ({}));
          const items = Array.isArray(d.items) ? d.items : [];
          if (!items.length) {
            listEl.innerHTML = `<div class="p3-empty">${_esc(_t("未找到市场插件"))}</div>`;
            return;
          }
          listEl.innerHTML = items.map((it) => {
            const done = it.installed || installedIds.has(it.id);
            return `<div class="p3-row">
              <div class="p3-row-main">
                <div class="p3-row-title">${_esc(it.name || it.id)}</div>
                <div class="p3-row-sub">${_esc(it.description || it.author || "")}</div>
              </div>
              ${done
                ? `<span class="p3-badge">${_esc(_t("已安装"))}</span>`
                : `<button type="button" class="p3-mini-btn primary" data-mkt-install="${_esc(it.id)}">${_esc(_t("安装"))}</button>`}
            </div>`;
          }).join("");
          listEl.querySelectorAll("[data-mkt-install]").forEach((btn) => {
            btn.addEventListener("click", async () => {
              const pluginId = btn.getAttribute("data-mkt-install");
              btn.disabled = true;
              const ir = await fetch("/gateway/plugins/install", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ pluginId, cascadeId: typeof activeCascadeId === "string" ? activeCascadeId : "", autoResume: false }),
              });
              if (ir.ok) {
                showToast?.(_t("插件安装成功"));
                onInstalled?.();
              } else {
                btn.disabled = false;
                showToast?.(_t("插件安装失败"));
              }
            });
          });
        } catch (_) {
          listEl.innerHTML = `<div class="p3-empty">${_esc(_t("加载插件市场失败"))}</div>`;
        }
      };

      document.getElementById("p4-mkt-go")?.addEventListener("click", () => load(document.getElementById("p4-mkt-q")?.value.trim()));
      document.getElementById("p4-mkt-q")?.addEventListener("keydown", (e) => {
        if (e.key === "Enter") load(e.target.value.trim());
      });
      load("");
    },

    // --- 9. Conversation Starring + Unified Long-Press Context ActionSheet ---
    openConvContextMenu(id, title) {
      if (!id) return;
      const item = typeof currentTrajectories === "object" ? currentTrajectories[id] : null;
      const isStarred = Boolean(item?.annotations?.starred);
      const isArchived = Boolean(item?.annotations?.archived);

      let overlay = document.getElementById("p4-conv-menu-overlay");
      if (!overlay) {
        overlay = document.createElement("div");
        overlay.id = "p4-conv-menu-overlay";
        overlay.className = "ios-actionsheet-overlay hidden";
        document.body.appendChild(overlay);
      }
      overlay.innerHTML = `
        <div class="ios-actionsheet-backdrop" id="p4-conv-menu-backdrop"></div>
        <div class="ios-actionsheet-container">
          <div class="ios-actionsheet-group">
            <div class="ios-actionsheet-header">
              <span class="actionsheet-title">${_esc(title || _t("会话操作"))}</span>
            </div>
            <button type="button" class="ios-actionsheet-btn" id="p4-cm-rename">${_esc(_t("重命名会话"))}</button>
            <button type="button" class="ios-actionsheet-btn" id="p4-cm-star">${_esc(isStarred ? _t("取消星标") : _t("星标会话"))}</button>
            <button type="button" class="ios-actionsheet-btn" id="p4-cm-archive">${_esc(isArchived ? _t("取消归档") : _t("归档会话"))}</button>
            <button type="button" class="ios-actionsheet-btn destructive" id="p4-cm-delete">${_esc(_t("删除会话"))}</button>
          </div>
          <div class="ios-actionsheet-group">
            <button type="button" class="ios-actionsheet-btn cancel" id="p4-cm-cancel">${_esc(_t("取消"))}</button>
          </div>
        </div>`;
      overlay.classList.remove("hidden");
      const close = () => overlay.classList.add("hidden");
      document.getElementById("p4-conv-menu-backdrop")?.addEventListener("click", close);
      document.getElementById("p4-cm-cancel")?.addEventListener("click", close);
      document.getElementById("p4-cm-rename")?.addEventListener("click", () => {
        close();
        if (typeof openRenameAlert === "function") openRenameAlert(id, title);
      });
      document.getElementById("p4-cm-star")?.addEventListener("click", () => {
        close();
        this.toggleStarConversation(id, !isStarred);
      });
      document.getElementById("p4-cm-archive")?.addEventListener("click", () => {
        close();
        if (typeof openArchiveActionSheet === "function") openArchiveActionSheet(id, !isArchived);
      });
      document.getElementById("p4-cm-delete")?.addEventListener("click", () => {
        close();
        if (typeof openDeleteActionSheet === "function") openDeleteActionSheet(id);
      });
    },

    async toggleStarConversation(id, starred) {
      if (!id) return;
      const prev = currentTrajectories?.[id]?.annotations?.starred;
      if (currentTrajectories && currentTrajectories[id]) {
        if (!currentTrajectories[id].annotations) currentTrajectories[id].annotations = {};
        currentTrajectories[id].annotations.starred = starred;
        if (typeof renderConversationList === "function") {
          renderConversationList(currentTrajectories, { forceReorder: false });
        }
      }
      try {
        const r = await fetch("/gateway/cascade/star", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ cascadeId: id, starred }),
        });
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        showToast?.(starred ? _t("已星标会话") : _t("已取消星标"));
      } catch (_) {
        if (currentTrajectories && currentTrajectories[id]?.annotations) {
          currentTrajectories[id].annotations.starred = prev;
          if (typeof renderConversationList === "function") {
            renderConversationList(currentTrajectories, { forceReorder: false });
          }
        }
        showToast?.(_t("星标操作失败"));
      }
    },
  };

  window.Phase4 = Phase4;
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", () => Phase4.init());
  } else {
    Phase4.init();
  }
})();
