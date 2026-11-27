// Phase 3 Native Feature Pack for Web & PWA
// Coordinates: @ context mentions, streaming dictation, permissions preset & amber badge,
// 5-tab Tools Board (MCP toggles, Skills + Memories CRUD, Plugins, Automations create/delete, Terminals/Tasks),
// Worktree/Clone, Suggested Responses, Fork, Narration playback, <agent-embed>/sidecar:// bridge, Git branch/discard.

(function () {
  const Phase3 = {
    permissionState: { preset: "strict", label: "严格审批", isUnlocked: false },
    contextItems: [], // Selected @ mentions for active draft
    mentionItems: [],
    mentionOpen: false,
    suggestedResponses: [],
    suggestedResponseSource: "",
    dismissedPredictedSuggestion: null,
    toolsTab: "mcp",
    memories: [],
    terminals: [],
    activeTerminalWs: null,
    activeTerminalId: "",
    terminalLogs: {},
    narrationAudio: null,
    dictateWs: null,
    dictateCtx: null,
    dictateStream: null,
    isDictating: false,
    workspaceNotes: (() => {
      try {
        return JSON.parse(localStorage.getItem("agy_workspace_notes") || "{}");
      } catch (_) {
        return {};
      }
    })(),

    init() {
      this.ensureOverlays();
      this.bindChatToolsButton();
      this.bindPermissionsSettings();
      this.bindNewSheetExtras();
      this.bindMentionTriggers();
      this.bindSuggestedResponsesVisibility();
      this.bindDictationLongPress();
      this.bindAgentEmbedBridge();
      this.bindSidecarLinks();
      this.fetchPermissions();
    },

    // --- 1. Permissions + Amber Dot Badge ---
    async fetchPermissions() {
      try {
        const res = await fetch("/gateway/permissions");
        if (!res.ok) return;
        const st = await res.json();
        this.applyPermissionState(st);
      } catch (_) {}
    },

    async setPermissionPreset(modeOrPreset) {
      const normalized = String(modeOrPreset || "strict").trim().toLowerCase();
      if (normalized === "turbo") {
        const msg = typeof t === "function"
          ? t("开启「全自动 · 无沙箱」后 Agent 将自动执行终端命令并访问工作区外文件（同步作用于桌面端全部会话）。确定开启吗？")
          : "开启「全自动 · 无沙箱」后 Agent 将自动执行终端命令并访问工作区外文件（同步作用于桌面端全部会话）。确定开启吗？";
        if (!confirm(msg)) {
          this.applyPermissionState(this.permissionState);
          return;
        }
      }
      const legacyPreset = normalized === "strict" ? "strict" : (normalized === "default" || normalized === "sandbox" ? "sandbox" : "auto");
      try {
        const res = await fetch("/gateway/permissions", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ mode: normalized, preset: legacyPreset }),
        });
        const st = await res.json();
        if (!res.ok || st.ok === false) throw new Error(st.error || "切换失败");
        this.applyPermissionState(st);
        const runningSuffix = typeof isCascadeRunning === "function" && activeCascadeId && isCascadeRunning(activeCascadeId) ? "（下轮生效）" : "";
        showToast?.(`权限档位已切换为「${st.label}」${runningSuffix}`);
      } catch (err) {
        showToast?.("切换权限失败: " + err.message, 2800);
      }
    },

    effectivePermissionMode(st) {
      const m = String(st?.mode || "").trim().toLowerCase();
      if (m === "strict" || m === "default" || m === "auto" || m === "turbo") return m;
      const p = String(st?.preset || "strict").trim().toLowerCase();
      if (p === "sandbox") return "default";
      if (p === "auto") return "turbo";
      return "strict";
    },

    applyPermissionState(st) {
      if (!st) return;
      this.permissionState = st;
      const mode = this.effectivePermissionMode(st);
      const dot = document.getElementById("btn-tools-perm-dot");
      if (dot) dot.classList.toggle("hidden", !st.isUnlocked && mode === "strict");
      const toolsBtn = document.getElementById("btn-chat-tools");
      if (toolsBtn) {
        const rawTitle = `工具与任务 (${st.label || "严格审批"}，长按/右键快切权限)`;
        toolsBtn.title = typeof t === "function" ? t(rawTitle) : rawTitle;
      }
      const sel = document.getElementById("settings-permission-preset");
      if (sel) sel.value = mode;
    },

    bindPermissionsSettings() {
      const sel = document.getElementById("settings-permission-preset");
      if (sel) {
        sel.addEventListener("change", (e) => {
          this.setPermissionPreset(e.target.value);
        });
      }
    },

    bindChatToolsButton() {
      const btn = document.getElementById("btn-chat-tools");
      if (!btn) return;
      let lpTimer = 0;
      let longPressed = false;

      const openPermMenu = (anchorRect) => {
        const menu = document.getElementById("phase3-perm-menu");
        if (!menu) return;
        const cur = this.effectivePermissionMode(this.permissionState);
        const presets = [
          { id: "strict", title: "严格审批", sub: "命令/非工作区文件每次确认" },
          { id: "default", title: "默认沙箱", sub: "沙箱内基础安全操作自动放行" },
          { id: "auto", title: "智能自动 · 沙箱", sub: "Antigravity 2.35 沙箱内智能放行" },
          { id: "turbo", title: "全自动 · 无沙箱", sub: "允许自动执行与跨区文件写入" },
        ];
        const isTurnRunning = typeof isCascadeRunning === "function" && activeCascadeId && isCascadeRunning(activeCascadeId);
        const runningHint = isTurnRunning ? `<div style="font-size:11px;color:#f59e0b;margin-top:2px;font-weight:500;">运行中 · 切换下轮生效</div>` : "";
        menu.innerHTML = `<div class="p3-pop-title">Agent 执行权限预设${runningHint}</div>` + presets.map((p) => `
          <button type="button" class="p3-pop-item ${cur === p.id ? "active" : ""}" data-preset="${p.id}">
            <div>
              <div class="p3-pop-label">${p.title}${cur === p.id ? " ✓" : ""}</div>
              <div class="p3-pop-sub">${p.sub}</div>
            </div>
          </button>`).join("");
        menu.classList.remove("hidden");
        const top = Math.min((anchorRect?.bottom || 56) + 8, window.innerHeight - 210);
        menu.style.top = `${top}px`;
        menu.style.right = "12px";
        menu.querySelectorAll("[data-preset]").forEach((el) => {
          el.onclick = () => {
            menu.classList.add("hidden");
            this.setPermissionPreset(el.getAttribute("data-preset"));
          };
        });
      };

      btn.addEventListener("touchstart", () => {
        longPressed = false;
        lpTimer = setTimeout(() => {
          longPressed = true;
          triggerHaptic?.("medium");
          openPermMenu(btn.getBoundingClientRect());
        }, 420);
      }, { passive: true });
      btn.addEventListener("touchend", () => clearTimeout(lpTimer));
      btn.addEventListener("touchcancel", () => clearTimeout(lpTimer));
      btn.addEventListener("contextmenu", (e) => {
        e.preventDefault();
        openPermMenu(btn.getBoundingClientRect());
      });
      btn.addEventListener("click", () => {
        if (longPressed) {
          longPressed = false;
          return;
        }
        this.openToolsBoard("mcp");
      });

      document.addEventListener("click", (e) => {
        const menu = document.getElementById("phase3-perm-menu");
        if (menu && !menu.classList.contains("hidden") && !e.target.closest("#phase3-perm-menu") && !e.target.closest("#btn-chat-tools")) {
          menu.classList.add("hidden");
        }
      });
    },

    // --- 2. 5-Tab Tools & Tasks Board (`MCP | 技能 | 插件 | 定时 | 任务`) ---
    ensureOverlays() {
      if (!document.getElementById("phase3-perm-menu")) {
        const pm = document.createElement("div");
        pm.id = "phase3-perm-menu";
        pm.className = "p3-popover-menu hidden";
        document.body.appendChild(pm);
      }
      if (!document.getElementById("sheet-tools")) {
        const sheet = document.createElement("div");
        sheet.id = "sheet-tools";
        sheet.className = "ios-sheet-overlay hidden";
        sheet.innerHTML = `
          <div class="ios-sheet-card p3-tools-card">
            <div class="sheet-grabber"></div>
            <div class="sheet-header">
              <button id="btn-tools-refresh" class="sheet-header-btn" type="button">刷新</button>
              <h2 class="sheet-title">工具与任务</h2>
              <button id="btn-tools-close" class="sheet-header-btn primary" type="button">完成</button>
            </div>
            <div class="p3-tab-bar" id="tools-tab-bar">
              <button type="button" class="p3-tab active" data-tab="mcp">MCP</button>
              <button type="button" class="p3-tab" data-tab="skills">技能</button>
              <button type="button" class="p3-tab" data-tab="plugins">插件</button>
              <button type="button" class="p3-tab" data-tab="automations">定时</button>
              <button type="button" class="p3-tab" data-tab="tasks">任务</button>
            </div>
            <div id="tools-tab-body" class="sheet-body p3-tools-body"></div>
          </div>`;
        document.body.appendChild(sheet);
        sheet.addEventListener("click", (e) => {
          if (e.target === sheet) this.closeToolsBoard();
        });
        document.getElementById("btn-tools-close")?.addEventListener("click", () => this.closeToolsBoard());
        document.getElementById("btn-tools-refresh")?.addEventListener("click", () => this.renderToolsTab(true));
        sheet.querySelectorAll("#tools-tab-bar .p3-tab").forEach((t) => {
          t.addEventListener("click", () => {
            this.toolsTab = t.getAttribute("data-tab") || "mcp";
            sheet.querySelectorAll("#tools-tab-bar .p3-tab").forEach((x) => x.classList.toggle("active", x === t));
            this.renderToolsTab(false);
          });
        });
      }
      if (!document.getElementById("narration-bar")) {
        const nb = document.createElement("div");
        nb.id = "narration-bar";
        nb.className = "p3-narration-bar hidden";
        nb.innerHTML = `
          <span class="p3-narr-title" id="narration-title">🔊 正在播报…</span>
          <button type="button" id="btn-narr-toggle" class="p3-narr-btn">暂停</button>
          <button type="button" id="btn-narr-close" class="p3-narr-btn">✕</button>`;
        document.body.appendChild(nb);
        document.getElementById("btn-narr-close")?.addEventListener("click", () => this.stopNarration());
        document.getElementById("btn-narr-toggle")?.addEventListener("click", () => {
          if (this.narrationAudio) {
            if (this.narrationAudio.paused) {
              this.narrationAudio.play();
              document.getElementById("btn-narr-toggle").textContent = "暂停";
            } else {
              this.narrationAudio.pause();
              document.getElementById("btn-narr-toggle").textContent = "继续";
            }
          }
        });
      }
      if (!document.getElementById("sheet-sidecar-web")) {
        const sc = document.createElement("div");
        sc.id = "sheet-sidecar-web";
        sc.className = "ios-sheet-overlay hidden";
        sc.innerHTML = `
          <div class="ios-sheet-card p3-sidecar-card">
            <div class="sheet-grabber"></div>
            <div class="sheet-header">
              <div style="width:48px"></div>
              <h2 class="sheet-title" id="sidecar-web-title">Sidecar 界面</h2>
              <button id="btn-sidecar-web-close" class="sheet-header-btn primary" type="button">关闭</button>
            </div>
            <iframe id="sidecar-web-frame" class="p3-sidecar-frame" src="about:blank"></iframe>
          </div>`;
        document.body.appendChild(sc);
        sc.addEventListener("click", (e) => {
          if (e.target === sc) this.closeSidecarSheet();
        });
        document.getElementById("btn-sidecar-web-close")?.addEventListener("click", () => this.closeSidecarSheet());
      }
      if (!document.getElementById("sheet-subagent-detail")) {
        const sd = document.createElement("div");
        sd.id = "sheet-subagent-detail";
        sd.className = "ios-sheet-overlay hidden";
        sd.innerHTML = `
          <div class="ios-sheet-card p3-subagent-card">
            <div class="sheet-grabber"></div>
            <div class="sheet-header">
              <div style="width:48px"></div>
              <h2 class="sheet-title" id="subagent-detail-title">子代理协作</h2>
              <button id="btn-subagent-detail-close" class="sheet-header-btn primary" type="button">完成</button>
            </div>
            <div id="subagent-detail-body" class="sheet-body p3-tools-body"></div>
          </div>`;
        document.body.appendChild(sd);
        sd.addEventListener("click", (e) => {
          if (e.target === sd) sd.classList.add("hidden");
        });
        document.getElementById("btn-subagent-detail-close")?.addEventListener("click", () => sd.classList.add("hidden"));
      }
    },

    openToolsBoard(tab = "mcp") {
      this.ensureOverlays();
      this.toolsTab = tab || "mcp";
      const sheet = document.getElementById("sheet-tools");
      if (!sheet) return;
      sheet.querySelectorAll("#tools-tab-bar .p3-tab").forEach((t) => {
        t.classList.toggle("active", t.getAttribute("data-tab") === this.toolsTab);
      });
      sheet.classList.remove("hidden");
      this.renderToolsTab(false);
    },

    closeToolsBoard() {
      document.getElementById("sheet-tools")?.classList.add("hidden");
      if (this.activeTerminalWs) {
        try { this.activeTerminalWs.close(); } catch (_) {}
        this.activeTerminalWs = null;
      }
    },

    async renderToolsTab(forceRefresh) {
      const body = document.getElementById("tools-tab-body");
      if (!body) return;
      body.innerHTML = `<div class="sheet-state"><div class="ios-spinner"></div><p>加载中…</p></div>`;

      try {
        if (this.toolsTab === "mcp") {
          if (forceRefresh) {
            await fetch("/gateway/mcp/refresh", { method: "POST" }).catch(() => {});
          }
          const res = await fetch("/gateway/mcp/states");
          const data = await res.json();
          const servers = Array.isArray(data.servers) ? data.servers : (Array.isArray(data) ? data : []);
          if (servers.length === 0) {
            body.innerHTML = `<div class="sheet-state"><p>暂无已配置的 MCP 服务器</p></div>`;
            return;
          }
          body.innerHTML = `<div class="p3-list">` + servers.map((sv) => {
            const name = sv.serverName || sv.name || "";
            const status = sv.status || "";
            const enabled = sv.enabled !== false && status !== "MCP_SERVER_STATUS_DISABLED";
            const toolsCount = Array.isArray(sv.tools) ? sv.tools.length : (sv.toolCount || 0);
            return `<div class="p3-row">
              <div class="p3-row-main">
                <div class="p3-row-title">${escapeHtml(name)}</div>
                <div class="p3-row-sub">${escapeHtml(status || (enabled ? "已启用" : "已停用"))}${toolsCount ? ` · ${toolsCount} 个工具` : ""}</div>
              </div>
              <div class="p3-row-actions">
                <button type="button" class="p3-mini-btn" data-mcp-oauth="${escapeHtml(name)}">OAuth</button>
                <label class="wf-switch"><input type="checkbox" data-mcp-toggle="${escapeHtml(name)}" ${enabled ? "checked" : ""}><span class="wf-switch-slider"></span></label>
              </div>
            </div>`;
          }).join("") + `</div>`;
          body.querySelectorAll("[data-mcp-oauth]").forEach((btn) => {
            btn.addEventListener("click", async () => {
              const serverName = btn.getAttribute("data-mcp-oauth");
              if (!serverName) return;
              const r = await fetch("/gateway/mcp/oauth/start", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ serverName }),
              });
              const d = await r.json().catch(() => ({}));
              if (d.ok || d.authUrl) {
                window.Phase4?.openOAuthHybrid?.({
                  title: `MCP OAuth · ${serverName}`,
                  startResponse: d,
                  mcpServerName: serverName,
                  onCompleted: () => this.renderToolsTab(true),
                });
              } else {
                showToast?.(d.error || "该 MCP 服务无需或不支持 OAuth", 2600);
              }
            });
          });
          body.querySelectorAll("[data-mcp-toggle]").forEach((cb) => {
            cb.addEventListener("change", async (e) => {
              const serverName = e.target.getAttribute("data-mcp-toggle");
              const enabled = !!e.target.checked;
              const r = await fetch("/gateway/mcp/toggle", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ serverName, enabled }),
              });
              if (!r.ok) {
                e.target.checked = !enabled;
                showToast?.("切换 MCP 失败");
              } else {
                showToast?.(`MCP「${serverName}」已${enabled ? "启用" : "停用"}`);
              }
            });
          });
          return;
        }

        if (this.toolsTab === "skills") {
          const wsUri = (typeof currentTrajectories === "object" && activeCascadeId && currentTrajectories[activeCascadeId]?.workspaceUris?.[0]) || "";
          const [scRes, memRes] = await Promise.all([
            fetch("/gateway/slash-commands"),
            fetch("/gateway/memories" + (wsUri ? `?workspaceUri=${encodeURIComponent(wsUri)}` : "")),
          ]);
          const scData = await scRes.json().catch(() => ({}));
          const memData = await memRes.json().catch(() => ({}));
          const commands = Array.isArray(scData.commands) ? scData.commands : [];
          this.memories = Array.isArray(memData.items) ? memData.items : [];

          const skillsHtml = commands.slice(0, 30).map((c) => `
            <div class="p3-row">
              <div class="p3-row-main">
                <div class="p3-row-title">/${escapeHtml(c.name)}</div>
                <div class="p3-row-sub">${escapeHtml(c.description || c.title || c.kind || "")}</div>
              </div>
            </div>`).join("");

          const memHtml = this.memories.map((m, idx) => `
            <div class="p3-row p3-mem-row" data-mem-idx="${idx}">
              <div class="p3-row-main">
                <div class="p3-row-title">${escapeHtml(m.title)} <span class="p3-badge">${escapeHtml(m.scope || "global")}</span></div>
                <div class="p3-row-sub monospaced">${escapeHtml((m.content || "").slice(0, 90).replace(/\n+/g, " "))}</div>
              </div>
              <div class="p3-row-actions">
                <button type="button" class="p3-mini-btn" data-mem-edit="${idx}">编辑</button>
                <button type="button" class="p3-mini-btn danger" data-mem-del="${idx}">删除</button>
              </div>
            </div>`).join("");

          body.innerHTML = `
            <div class="p3-section-header">
              <span>Agent 记忆与规则库 (Memories &amp; Rules)</span>
              <button type="button" class="p3-mini-btn primary" id="btn-mem-add">+ 新增记忆/规则</button>
            </div>
            <div id="p3-mem-editor" class="p3-form-card hidden">
              <input type="text" id="p3-mem-title" class="p3-input" placeholder="规则/记忆标题 (如 coding-style)" />
              <select id="p3-mem-scope" class="p3-input">
                <option value="global">全局规则 (~/.agents/rules)</option>
                <option value="workspace">当前工作区规则 (.agents/rules)</option>
                <option value="memory">持久记忆 (memories)</option>
              </select>
              <textarea id="p3-mem-content" class="p3-textarea" rows="4" placeholder="输入约束或记忆内容…"></textarea>
              <div class="p3-form-actions">
                <button type="button" class="p3-mini-btn" id="btn-mem-cancel">取消</button>
                <button type="button" class="p3-mini-btn primary" id="btn-mem-save">保存</button>
              </div>
            </div>
            <div class="p3-list">${memHtml || '<div class="p3-empty">暂无规则与记忆文件</div>'}</div>
            <div class="p3-section-header" style="margin-top:16px;">可用斜杠命令与技能 (${commands.length})</div>
            <div class="p3-list">${skillsHtml || '<div class="p3-empty">暂无可用技能</div>'}</div>
          `;

          let editingPath = "";
          const editor = document.getElementById("p3-mem-editor");
          document.getElementById("btn-mem-add")?.addEventListener("click", () => {
            editingPath = "";
            document.getElementById("p3-mem-title").value = "";
            document.getElementById("p3-mem-content").value = "";
            editor.classList.remove("hidden");
          });
          document.getElementById("btn-mem-cancel")?.addEventListener("click", () => editor.classList.add("hidden"));
          document.getElementById("btn-mem-save")?.addEventListener("click", async () => {
            const title = document.getElementById("p3-mem-title").value.trim();
            const scope = document.getElementById("p3-mem-scope").value;
            const content = document.getElementById("p3-mem-content").value;
            const r = await fetch("/gateway/memories", {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ action: "upsert", path: editingPath, scope, workspacePath: wsUri, title, content }),
            });
            const resp = await r.json();
            if (!r.ok || resp.ok === false) {
              showToast?.("保存失败: " + (resp.error || r.status));
              return;
            }
            showToast?.("记忆/规则已保存");
            this.renderToolsTab(false);
          });
          body.querySelectorAll("[data-mem-edit]").forEach((btn) => {
            btn.addEventListener("click", () => {
              const item = this.memories[Number(btn.getAttribute("data-mem-edit"))];
              if (!item) return;
              editingPath = item.path || "";
              document.getElementById("p3-mem-title").value = item.title || "";
              document.getElementById("p3-mem-scope").value = item.scope || "global";
              document.getElementById("p3-mem-content").value = item.content || "";
              editor.classList.remove("hidden");
            });
          });
          body.querySelectorAll("[data-mem-del]").forEach((btn) => {
            btn.addEventListener("click", async () => {
              const item = this.memories[Number(btn.getAttribute("data-mem-del"))];
              if (!item || !confirm(typeof t === "function" ? t(`确定删除「${item.title}」？`) : `确定删除「${item.title}」？`)) return;
              const r = await fetch("/gateway/memories", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ action: "delete", path: item.path, workspacePath: wsUri }),
              });
              if (r.ok) {
                showToast?.("已删除");
                this.renderToolsTab(false);
              }
            });
          });
          return;
        }

        if (this.toolsTab === "plugins") {
          const res = await fetch("/gateway/plugins");
          const data = await res.json();
          const list = Array.isArray(data.plugins) ? data.plugins : (Array.isArray(data) ? data : []);
          const providers = Array.isArray(data.providers) ? data.providers : [];
          const grants = Array.isArray(data.permissionGrants) ? data.permissionGrants : [];
          const scopeLabel = (g) => {
            if (g.scope === "PLUGIN_PERMISSION_GRANT_SCOPE_GLOBAL") return "全局";
            if (g.scope === "PLUGIN_PERMISSION_GRANT_SCOPE_PROJECT") return g.projectName || "项目";
            if (g.scope === "PLUGIN_PERMISSION_GRANT_SCOPE_DEFAULT") return "默认";
            return "已授权";
          };
          const providersHtml = providers.length
            ? `<div class="p3-section-header" style="margin-bottom:6px;"><span>插件账号与认证 (${providers.length})</span></div>
               <div class="p3-list" style="margin-bottom:12px;">` +
              providers.map((pv, idx) => {
                const label = pv.name || pv.provider || pv.id || "";
                const sub = pv.identity || pv.needsAuthReason || (pv.signedIn ? "已连接" : "未登录");
                const accounts = Array.isArray(pv.accounts) ? pv.accounts : [];
                const accChips = accounts.length > 1
                  ? `<div style="display:flex;gap:6px;flex-wrap:wrap;margin-top:6px;">` +
                    accounts.map((acc) => `<button type="button" class="p3-mini-btn ${acc.isDefault ? "primary" : ""}" data-pv-acc="${idx}" data-acc-id="${escapeHtml(acc.accountId || "")}">${escapeHtml(acc.label || acc.identity || acc.alias || acc.accountId || "")}</button>`).join("") +
                    `</div>`
                  : "";
                return `<div class="p3-row" style="flex-direction:column;align-items:stretch;">
                  <div style="display:flex;align-items:center;justify-content:space-between;gap:8px;">
                    <div class="p3-row-main">
                      <div class="p3-row-title">${escapeHtml(label)}</div>
                      <div class="p3-row-sub">${escapeHtml(sub)}</div>
                    </div>
                    ${pv.signedIn
                      ? `<button type="button" class="p3-mini-btn danger" data-pv-signout="${idx}">退出登录</button>`
                      : `<button type="button" class="p3-mini-btn primary" data-pv-signin="${idx}">登录</button>`}
                  </div>
                  ${accChips}
                </div>`;
              }).join("") + `</div>`
            : "";
          const grantsHtml = grants.length
            ? `<div class="p3-section-header" style="margin-bottom:6px;"><span>已授权插件权限 (${grants.length})</span></div>
               <div class="p3-list" style="margin-bottom:12px;">` +
              grants.map((g, idx) => {
                const title = g.description || [g.plugin || g.pluginId, g.actionLabel || g.action].filter(Boolean).join(" · ") || g.grant;
                const sub = g.resourceName || (g.anyResource ? "全部资源" : "");
                return `<div class="p3-row">
                  <div class="p3-row-main">
                    <div class="p3-row-title">${escapeHtml(title)} <span class="p3-badge">${escapeHtml(scopeLabel(g))}</span></div>
                    ${sub ? `<div class="p3-row-sub monospaced">${escapeHtml(sub)}</div>` : ""}
                  </div>
                  <button type="button" class="p3-mini-btn" data-revoke-grant="${idx}" style="color:#ef4444;">撤销</button>
                </div>`;
              }).join("") +
              `</div>`
            : "";
          body.innerHTML = providersHtml + grantsHtml + `<div class="p3-section-header" style="margin-bottom:6px;"><span>已安装插件 (${list.length})</span></div><div class="p3-list">` + (list.map((pl) => {
            const pid = pl.id || pl.name || "";
            return `
            <div class="p3-row">
              <div class="p3-row-main">
                <div class="p3-row-title">${escapeHtml(pl.name || pl.id || "")}</div>
                <div class="p3-row-sub">${escapeHtml(pl.description || pl.version || "")}</div>
              </div>
              <div class="p3-row-actions">
                <span class="p3-badge">${pl.enabled === false ? "停用" : "启用"}</span>
                ${pl.installed !== false && pid ? `<button type="button" class="p3-mini-btn danger" data-plugin-del="${escapeHtml(pid)}">卸载</button>` : ""}
              </div>
            </div>`;
          }).join("") || `<div class="p3-empty">暂无已安装插件</div>`) + `</div><div id="p4-marketplace-mount"></div>`;

          body.querySelectorAll("[data-pv-signin]").forEach((btn) => {
            btn.addEventListener("click", async () => {
              const pv = providers[Number(btn.getAttribute("data-pv-signin"))];
              if (!pv) return;
              const plugin = pv.plugin || (pv.id || "").split(":")[0] || "";
              const provider = pv.provider || ((pv.id || "").includes(":") ? pv.id.split(":").slice(1).join(":") : pv.id || pv.name);
              const r = await fetch("/gateway/plugins/auth/start", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ plugin, provider }),
              });
              const d = await r.json().catch(() => ({}));
              if (d.ok || d.authUrl) {
                window.Phase4?.openOAuthHybrid?.({
                  title: typeof t === "function" ? t(`${pv.name || provider} 登录`) : `${pv.name || provider} 登录`,
                  startResponse: d,
                  onCompleted: () => this.renderToolsTab(false),
                });
              } else {
                showToast?.(d.error || (typeof t === "function" ? t("发起登录失败") : "发起登录失败"), 2600);
              }
            });
          });
          body.querySelectorAll("[data-pv-signout]").forEach((btn) => {
            btn.addEventListener("click", async () => {
              const pv = providers[Number(btn.getAttribute("data-pv-signout"))];
              if (!pv) return;
              const plugin = pv.plugin || (pv.id || "").split(":")[0] || "";
              const provider = pv.provider || ((pv.id || "").includes(":") ? pv.id.split(":").slice(1).join(":") : pv.id || pv.name);
              await fetch("/gateway/plugins/auth/signout", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ plugin, provider }),
              });
              showToast?.(typeof t === "function" ? t("已退出登录") : "已退出登录");
              this.renderToolsTab(false);
            });
          });
          body.querySelectorAll("[data-pv-acc]").forEach((btn) => {
            btn.addEventListener("click", async () => {
              const pv = providers[Number(btn.getAttribute("data-pv-acc"))];
              const accountId = btn.getAttribute("data-acc-id");
              if (!pv || !accountId) return;
              const plugin = pv.plugin || (pv.id || "").split(":")[0] || "";
              const provider = pv.provider || ((pv.id || "").includes(":") ? pv.id.split(":").slice(1).join(":") : pv.id || pv.name);
              await fetch("/gateway/plugins/auth/default", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ plugin, provider, accountId }),
              });
              this.renderToolsTab(false);
            });
          });
          body.querySelectorAll("[data-plugin-del]").forEach((btn) => {
            btn.addEventListener("click", async () => {
              const pluginId = btn.getAttribute("data-plugin-del");
              const confirmMsg = typeof t === "function" ? t(`确定卸载插件「${pluginId}」？`) : `确定卸载插件「${pluginId}」？`;
              if (!pluginId || !confirm(confirmMsg)) return;
              const r = await fetch("/gateway/plugins/delete", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ pluginId }),
              });
              if (r.ok) {
                showToast?.(typeof t === "function" ? t("插件已卸载") : "插件已卸载");
                this.renderToolsTab(false);
              } else {
                showToast?.(typeof t === "function" ? t("卸载失败") : "卸载失败");
              }
            });
          });
          body.querySelectorAll("[data-revoke-grant]").forEach((btn) => {
            btn.addEventListener("click", async () => {
              const g = grants[Number(btn.getAttribute("data-revoke-grant"))];
              if (!g) return;
              btn.disabled = true;
              try {
                const r = await fetch("/gateway/plugins/grants/revoke", {
                  method: "POST",
                  headers: { "Content-Type": "application/json" },
                  body: JSON.stringify({ grant: g.grant, scope: g.scope || "", projectId: g.projectId || "" }),
                });
                if (!r.ok) throw new Error(`HTTP ${r.status}`);
                showToast?.("已撤销插件授权");
                this.renderToolsTab(false);
              } catch (err) {
                btn.disabled = false;
                showToast?.("撤销失败: " + (err.message || err), 2600);
              }
            });
          });
          const installedIds = new Set(list.filter((pl) => pl.installed !== false).map((pl) => pl.id || pl.name));
          window.Phase4?.renderMarketplaceSection?.(document.getElementById("p4-marketplace-mount"), installedIds, () => this.renderToolsTab(false));
          return;
        }

        if (this.toolsTab === "automations") {
          const res = await fetch("/gateway/automations");
          const data = await res.json();
          const list = Array.isArray(data.schedules) ? data.schedules : (Array.isArray(data.automations) ? data.automations : []);
          const events = Array.isArray(data.events) ? data.events : [];
          this._automationsCache = list;
          this._automationEventsCache = events;

          if (this._creatingAutomation) {
            await this.renderCreateAutomationForm(body, list);
            return;
          }

          if (this._selectedAutomationId) {
            const target = list.find((s) => s.id === this._selectedAutomationId);
            if (target) {
              const openPrompt = !!this._openAutomationPromptEditor;
              this._openAutomationPromptEditor = false;
              await this.renderAutomationDetail(body, target, events, openPrompt);
              return;
            }
            this._selectedAutomationId = null;
            this._openAutomationPromptEditor = false;
          }

          const isAutoRunning = (sc) => !!(sc.enabled || /^(running|ready)$/i.test(sc.status || ""));
          const scheduled = list
            .filter(isAutoRunning)
            .sort((a, b) => {
              const diff = this._parseAutoNextRunMs(a.nextRunAt) - this._parseAutoNextRunMs(b.nextRunAt);
              return diff !== 0 ? diff : String(a.name || a.id).localeCompare(String(b.name || b.id));
            });
          const paused = list
            .filter((sc) => !isAutoRunning(sc))
            .sort((a, b) => String(a.name || a.id).localeCompare(String(b.name || b.id)));

          const renderAutoCard = (sc) => {
            const running = isAutoRunning(sc);
            const hasErr = /^error$/i.test(sc.status || "") || !!sc.lastError;
            const summary = this._humanizeAutoCron(sc.cronExpression || "");
            const preview = (sc.description || sc.prompt || "").replace(/\s+/g, " ").trim().slice(0, 100);
            const nextLbl = typeof t === "function" ? t("下次") : "下次";
            const lastLbl = typeof t === "function" ? t("上次") : "上次";
            const runLine = [
              running && sc.nextRunAt ? `${nextLbl} ${sc.nextRunAt}` : "",
              sc.lastRunAt ? `${lastLbl} ${sc.lastRunAt}` : "",
            ].filter(Boolean).join(" · ");
            return `
              <div class="p3-row p3-auto-card" data-auto-open="${escapeHtml(sc.id)}">
                <div class="p3-row-main">
                  <div class="p3-row-title">
                    <span>${escapeHtml(sc.name || sc.id)}</span>
                    ${hasErr ? `<span class="p3-badge p3-auto-err-badge">异常</span>` : ""}
                  </div>
                  <div class="p3-row-sub p3-auto-meta">
                    ${summary ? `<span>⏱ ${escapeHtml(summary)}</span>` : ""}
                    ${sc.projectName ? `<span>· 📁 ${escapeHtml(sc.projectName)}</span>` : ""}
                  </div>
                  ${runLine ? `<div class="p3-row-sub monospaced" style="font-size:11px;opacity:.72;">${escapeHtml(runLine)}</div>` : ""}
                  ${preview ? `<div class="p3-row-sub p3-auto-preview">${escapeHtml(preview)}</div>` : ""}
                </div>
                <div class="p3-row-actions p3-auto-actions">
                  ${running ? `<button type="button" class="p3-mini-btn p3-auto-icon-btn" data-auto-run="${escapeHtml(sc.id)}" title="立即运行">▶</button>` : ""}
                  <button type="button" class="p3-mini-btn p3-auto-icon-btn" data-auto-edit-prompt="${escapeHtml(sc.id)}" title="编辑执行指令">✎</button>
                  <label class="wf-switch" title="${running ? "暂停" : "启用"}">
                    <input type="checkbox" data-auto-toggle="${escapeHtml(sc.id)}" ${running ? "checked" : ""}>
                    <span class="wf-switch-slider"></span>
                  </label>
                </div>
              </div>`;
          };

          const renderEventsHtml = () => {
            if (!events.length) return "";
            const statusDot = (ev) => {
              const st = ev.runStatus || (ev.status === "error" ? "failed" : "ok");
              return `<span class="p3-auto-status-dot ${escapeHtml(st)}"></span>`;
            };
            return `
              <div class="p3-section-header" style="margin-top:14px;">
                <span>最近触发记录</span>
              </div>
              <div class="p3-list">
                ${events.slice(0, 12).map((ev) => {
                  const title = ev.conversationTitle || ev.scheduleId || "自动化执行";
                  const sub = [ev.conversationTitle && ev.scheduleId && ev.scheduleId !== ev.conversationTitle ? ev.scheduleId : "", ev.message || ""].filter(Boolean).join(" · ").slice(0, 110);
                  return `
                    <div class="p3-row" ${ev.conversationId ? `data-auto-conv="${escapeHtml(ev.conversationId)}" data-auto-conv-exists="${ev.conversationExists === false ? "0" : "1"}" style="cursor:pointer;"` : ""}>
                      <div class="p3-row-main">
                        <div class="p3-row-title" style="font-size:13px;gap:6px;display:flex;align-items:center;">
                          ${statusDot(ev)}
                          <span>${escapeHtml(title)}</span>
                        </div>
                        <div class="p3-row-sub monospaced" style="font-size:11px;opacity:.7;">${escapeHtml(ev.timestamp || "")}</div>
                        ${sub ? `<div class="p3-row-sub">${escapeHtml(sub)}</div>` : ""}
                      </div>
                      ${ev.conversationId ? `<span style="opacity:.45;font-size:14px;">›</span>` : ""}
                    </div>`;
                }).join("")}
              </div>`;
          };

          body.innerHTML = `
            <div class="p3-auto-create-bar">
              <button type="button" class="p3-mini-btn primary p3-auto-prompt-btn" id="btn-auto-create-prompt">✨ 通过提示词创建</button>
              <button type="button" class="p3-mini-btn p3-auto-manual-btn" id="btn-auto-new">+ 手动新建</button>
            </div>
            ${list.length === 0 ? `
              <div class="p3-empty" style="padding:24px 12px;">
                <div style="font-size:14px;font-weight:600;margin-bottom:6px;color:var(--ios-label);">暂无定时自动化任务</div>
                <div style="font-size:12px;opacity:.78;">描述你想自动化的事项让 AI 帮你设计，或手动配置 Cron 计划</div>
              </div>
            ` : `
              ${scheduled.length > 0 ? `
                <div class="p3-section-header"><span>计划运行 (${scheduled.length})</span></div>
                <div class="p3-list">${scheduled.map(renderAutoCard).join("")}</div>
              ` : ""}
              ${paused.length > 0 ? `
                <div class="p3-section-header" style="${scheduled.length > 0 ? "margin-top:14px;" : ""}"><span>已暂停 (${paused.length})</span></div>
                <div class="p3-list">${paused.map(renderAutoCard).join("")}</div>
              ` : ""}
            `}
            ${renderEventsHtml()}`;

          document.getElementById("btn-auto-create-prompt")?.addEventListener("click", () => {
            this.startAutomationWithPrompt();
          });
          document.getElementById("btn-auto-new")?.addEventListener("click", () => {
            this._creatingAutomation = true;
            this.renderToolsTab(false);
          });
          body.querySelectorAll("[data-auto-open]").forEach((row) => {
            row.addEventListener("click", (e) => {
              if (e.target.closest("button, label, input")) return;
              this._selectedAutomationId = row.getAttribute("data-auto-open");
              this._openAutomationPromptEditor = false;
              this.renderToolsTab(false);
            });
          });
          body.querySelectorAll("[data-auto-edit-prompt]").forEach((btn) => {
            btn.addEventListener("click", (e) => {
              e.stopPropagation();
              this._selectedAutomationId = btn.getAttribute("data-auto-edit-prompt");
              this._openAutomationPromptEditor = true;
              this.renderToolsTab(false);
            });
          });
          body.querySelectorAll("[data-auto-toggle]").forEach((cb) => {
            cb.addEventListener("change", async (e) => {
              e.stopPropagation();
              const sidecarId = cb.getAttribute("data-auto-toggle");
              const wantStart = !!cb.checked;
              cb.disabled = true;
              const r = await fetch("/gateway/automations/manage", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ sidecarId, action: wantStart ? "start" : "stop" }),
              });
              const d = await r.json().catch(() => ({}));
              if (!r.ok) {
                cb.checked = !wantStart;
                cb.disabled = false;
                showToast?.("操作失败: " + (d.error || r.status));
                return;
              }
              showToast?.(wantStart ? "已启动定时任务" : "已暂停定时任务");
              this.renderToolsTab(false);
            });
          });
          body.querySelectorAll("[data-auto-run]").forEach((btn) => {
            btn.addEventListener("click", async (e) => {
              e.stopPropagation();
              const sidecarId = btn.getAttribute("data-auto-run");
              btn.disabled = true;
              const r = await fetch("/gateway/automations/manage", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ sidecarId, action: "run_now" }),
              });
              const d = await r.json().catch(() => ({}));
              btn.disabled = false;
              if (!r.ok) {
                showToast?.("立即运行失败: " + (d.error || r.status));
                return;
              }
              showToast?.("已触发立即执行");
              this.renderToolsTab(false);
            });
          });
          body.querySelectorAll("[data-auto-conv]").forEach((row) => {
            row.addEventListener("click", () => {
              const cid = row.getAttribute("data-auto-conv");
              const exists = row.getAttribute("data-auto-conv-exists") !== "0";
              if (!exists) {
                showToast?.(typeof t === "function" ? t("该触发记录对应的会话已删除") : "该触发记录对应的会话已删除", 2400);
                return;
              }
              const openConv = typeof selectCascadeConversation === "function"
                ? selectCascadeConversation
                : (typeof selectConversation === "function" ? selectConversation : null);
              if (cid && openConv) {
                this.closeToolsBoard();
                openConv(cid);
              }
            });
          });
          return;
        }

        if (this.toolsTab === "tasks") {
          const [bgRes, termRes] = await Promise.all([
            fetch("/gateway/tasks/background").catch(() => null),
            fetch("/gateway/terminals").catch(() => null),
          ]);
          const bgData = bgRes ? await bgRes.json().catch(() => ({})) : {};
          const termData = termRes ? await termRes.json().catch(() => ({})) : {};
          const bgTasks = Array.isArray(bgData.tasks) ? bgData.tasks : [];
          this.terminals = Array.isArray(termData.terminals) ? termData.terminals : [];

          body.innerHTML = `
            <div class="p3-section-header">
              <span>交互式终端 (${this.terminals.length})</span>
              <button type="button" class="p3-mini-btn primary" id="btn-term-create">+ 新建终端</button>
            </div>
            <div id="p3-terminals-list" class="p3-list">
              ${this.terminals.map((t) => {
                const tid = t.terminalId || t.id || "";
                const title = t.name || t.title || t.command || tid.slice(0, 8);
                return `<div class="p3-row">
                  <div class="p3-row-main">
                    <div class="p3-row-title">${escapeHtml(title)}</div>
                    <div class="p3-row-sub monospaced">${escapeHtml(tid)}</div>
                  </div>
                  <div class="p3-row-actions">
                    <button type="button" class="p3-mini-btn primary" data-term-open="${escapeHtml(tid)}">交互控制台</button>
                    <button type="button" class="p3-mini-btn danger" data-term-close="${escapeHtml(tid)}">关闭</button>
                  </div>
                </div>`;
              }).join("") || `<div class="p3-empty">暂无活跃上游终端</div>`}
            </div>
            <div id="p3-terminal-drawer" class="p3-terminal-drawer ${this.activeTerminalId ? "" : "hidden"}">
              <div class="p3-terminal-head">
                <span class="monospaced" id="p3-term-active-label">终端: ${escapeHtml(this.activeTerminalId)}</span>
                <div class="p3-term-quick">
                  <button type="button" class="p3-mini-btn" data-term-key="y&#10;">y</button>
                  <button type="button" class="p3-mini-btn" data-term-key="n&#10;">n</button>
                  <button type="button" class="p3-mini-btn" data-term-key="&#10;">Enter</button>
                  <button type="button" class="p3-mini-btn danger" data-term-key="&#x03;">Ctrl+C</button>
                </div>
              </div>
              <pre id="p3-term-log" class="p3-term-log">${escapeHtml(this.terminalLogs[this.activeTerminalId] || "等待输出…")}</pre>
              <div class="p3-term-input-row">
                <input type="text" id="p3-term-input" class="p3-input monospaced" placeholder="向终端发送输入并回车…" />
                <button type="button" id="p3-term-send" class="p3-mini-btn primary">发送</button>
              </div>
            </div>
            <div class="p3-section-header" style="margin-top:16px;">全局后台任务 (${bgTasks.length})</div>
            <div class="p3-list">
              ${bgTasks.map((tk) => `<div class="p3-row">
                <div class="p3-row-main">
                  <div class="p3-row-title">${escapeHtml(tk.title || tk.toolName || tk.commandLine || "任务")}</div>
                  <div class="p3-row-sub">${escapeHtml(tk.commandLine || "")}</div>
                </div>
                ${tk.conversationId ? `<button type="button" class="p3-mini-btn" data-jump-cid="${escapeHtml(tk.conversationId)}">打开</button>` : ""}
              </div>`).join("") || `<div class="p3-empty">暂无后台任务</div>`}
            </div>`;

          document.getElementById("btn-term-create")?.addEventListener("click", async () => {
            const r = await fetch("/gateway/terminal/create", { method: "POST", headers: { "Content-Type": "application/json" }, body: "{}" });
            const d = await r.json().catch(() => ({}));
            if (d.terminalId) this.openTerminalStream(d.terminalId);
            this.renderToolsTab(false);
          });
          body.querySelectorAll("[data-term-open]").forEach((b) => {
            b.addEventListener("click", () => this.openTerminalStream(b.getAttribute("data-term-open")));
          });
          body.querySelectorAll("[data-term-close]").forEach((b) => {
            b.addEventListener("click", async () => {
              await fetch("/gateway/terminal/close", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ terminalId: b.getAttribute("data-term-close") }),
              });
              this.renderToolsTab(false);
            });
          });
          body.querySelectorAll("[data-term-key]").forEach((b) => {
            b.addEventListener("click", () => this.sendTerminalInput(b.getAttribute("data-term-key")));
          });
          document.getElementById("p3-term-send")?.addEventListener("click", () => {
            const inp = document.getElementById("p3-term-input");
            if (inp && inp.value !== "") {
              this.sendTerminalInput(inp.value + "\n");
              inp.value = "";
            }
          });
          document.getElementById("p3-term-input")?.addEventListener("keydown", (e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              document.getElementById("p3-term-send")?.click();
            }
          });
          body.querySelectorAll("[data-jump-cid]").forEach((b) => {
            b.addEventListener("click", () => {
              const cid = b.getAttribute("data-jump-cid");
              this.closeToolsBoard();
              navigateTo?.(`#c=${cid}`);
            });
          });
        }
      } catch (err) {
        body.innerHTML = `<div class="sheet-state warn"><p>加载失败: ${escapeHtml(err.message)}</p></div>`;
      }
    },

    async acquireWsTicketQueryParam(sep = "&") {
      try {
        const doFetch = typeof originalFetch === "function" ? originalFetch : fetch;
        const resp = await doFetch("/api/v1/auth/ws-ticket", { method: "POST" });
        if (resp && resp.ok) {
          const data = await resp.json();
          if (data && data.ticket) {
            return `${sep}ticket=${encodeURIComponent(data.ticket)}`;
          }
        }
      } catch (_) {}
      return "";
    },

    async openTerminalStream(terminalId) {
      if (!terminalId) return;
      this.activeTerminalId = terminalId;
      const mySeq = (this._termStreamSeq = (this._termStreamSeq || 0) + 1);
      document.getElementById("p3-terminal-drawer")?.classList.remove("hidden");
      const label = document.getElementById("p3-term-active-label");
      if (label) label.textContent = `终端: ${terminalId}`;
      if (this.activeTerminalWs) {
        try { this.activeTerminalWs.close(); } catch (_) {}
        this.activeTerminalWs = null;
      }
      const ticketParam = await this.acquireWsTicketQueryParam("&");
      if (mySeq !== this._termStreamSeq || this.activeTerminalId !== terminalId) return;
      const proto = location.protocol === "https:" ? "wss:" : "ws:";
      const ws = new WebSocket(`${proto}//${location.host}/gateway/terminal/stream?terminalId=${encodeURIComponent(terminalId)}${ticketParam}`);
      this.activeTerminalWs = ws;
      ws.onmessage = (ev) => {
        try {
          const msg = JSON.parse(ev.data);
          const chunk = msg.output || msg.data || msg.chunk || "";
          if (chunk) {
            this.terminalLogs[terminalId] = ((this.terminalLogs[terminalId] || "") + chunk).slice(-8000);
            const pre = document.getElementById("p3-term-log");
            if (pre) {
              pre.textContent = this.terminalLogs[terminalId];
              pre.scrollTop = pre.scrollHeight;
            }
          }
        } catch (_) {}
      };
    },

    async sendTerminalInput(input) {
      if (!this.activeTerminalId) return;
      await fetch("/gateway/terminal/input", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          terminalId: this.activeTerminalId,
          input,
          conversationId: activeCascadeId || "",
        }),
      });
    },

    // --- 3. New Conversation Sheet: Worktree / Workspace Alias + Visual GitHub Clone & Add Workspace ---
    _newSheetSubState: null,

    resetNewSheetSubView() {
      const st = this._newSheetSubState;
      if (st?.pollTimer) {
        clearInterval(st.pollTimer);
        st.pollTimer = 0;
      }
      this._newSheetSubState = null;
      document.getElementById("sheet-new-main")?.classList.remove("hidden");
      const subEl = document.getElementById("sheet-new-sub");
      if (subEl) {
        subEl.classList.add("hidden");
        subEl.innerHTML = "";
      }
    },

    parseGitHubRepoSlug(raw) {
      let s = (raw || "").trim();
      if (!s || s.startsWith("-")) return null;
      let fromUrl = false;
      if (s.startsWith("git@github.com:")) {
        s = s.slice("git@github.com:".length);
        fromUrl = true;
      } else if (s.startsWith("https://github.com/")) {
        s = s.slice("https://github.com/".length);
        fromUrl = true;
      } else if (s.startsWith("http://github.com/")) {
        s = s.slice("http://github.com/".length);
        fromUrl = true;
      } else if (s.includes("://") || s.includes("@")) {
        return null;
      }
      const cut = s.search(/[?#]/);
      if (cut >= 0) s = s.slice(0, cut);
      s = s.replace(/^\/+|\/+$/g, "");
      const parts = s.split("/").filter(Boolean);
      if (parts.length < 2 || (!fromUrl && parts.length !== 2)) return null;
      const owner = parts[0];
      const repo = parts[1].replace(/\.git$/, "");
      if (owner === "." || owner === ".." || repo === "." || repo === "..") return null;
      const segRe = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/;
      if (!segRe.test(owner) || !segRe.test(repo)) return null;
      return { slug: `${owner}/${repo}`, repoName: repo };
    },

    async _refreshDiscoveredProjectsList(createdProject) {
      if (createdProject && Array.isArray(discoveredProjects)) {
        discoveredProjects = discoveredProjects.filter(
          (p) => p.id !== createdProject.id && p.uri !== createdProject.uri && p.path !== createdProject.path
        );
        discoveredProjects.unshift(createdProject);
        renderNewProjectsList?.(discoveredProjects);
      }
      try {
        const listRes = await fetch("/gateway/projects");
        if (listRes.ok) {
          discoveredProjects = await listRes.json();
          renderNewProjectsList?.(discoveredProjects);
        }
      } catch (_) {}
    },

    _openNewSheetSubView(mode) {
      const mainEl = document.getElementById("sheet-new-main");
      const subEl = document.getElementById("sheet-new-sub");
      if (!mainEl || !subEl) return;

      if (this._newSheetSubState?.pollTimer) {
        clearInterval(this._newSheetSubState.pollTimer);
      }

      const state = {
        mode, // "clone" | "add"
        phase: "form", // "form" | "running" | "done"
        repoInput: "",
        customFolderName: "",
        showFolderRename: false,
        dirLoading: true,
        dirCreating: false,
        dirError: "",
        currentPath: "",
        currentDisplay: "",
        parentPath: "",
        canGoUp: false,
        recommendedDirs: [],
        entries: [],
        submitting: false,
        error: "",
        conflictAlreadyClonedPath: "",
        jobId: "",
        progress: 0,
        progressMessage: "",
        targetPath: "",
        completedProject: null,
        completedRepoSlug: "",
        completedAlreadyExists: false,
        pollTimer: 0,
      };
      this._newSheetSubState = state;
      mainEl.classList.add("hidden");
      subEl.classList.remove("hidden");

      const loadDirs = async (targetPath) => {
        state.dirLoading = true;
        state.dirError = "";
        this._renderNewSheetSubView();
        try {
          const qs = targetPath ? `?path=${encodeURIComponent(targetPath)}` : "";
          const res = await fetch(`/gateway/fs/dirs${qs}`);
          const d = await res.json();
          if (!res.ok || !d.ok) throw new Error(d.error || "读取本机目录失败");
          state.currentPath = d.currentPath || "";
          state.currentDisplay = d.currentDisplay || d.currentPath || "";
          state.parentPath = d.parentPath || "";
          state.canGoUp = !!d.canGoUp;
          state.recommendedDirs = Array.isArray(d.recommendedDirs) ? d.recommendedDirs : [];
          state.entries = Array.isArray(d.entries) ? d.entries : [];
          state.error = "";
          state.conflictAlreadyClonedPath = "";
        } catch (err) {
          state.dirError = err.message || "读取本机目录失败";
        } finally {
          state.dirLoading = false;
          if (this._newSheetSubState === state) this._renderNewSheetSubView();
        }
      };

      const createDir = async () => {
        const raw = prompt(
          typeof t === "function"
            ? t(`在 ${state.currentDisplay || "~"} 下新建目录名称，或输入 ~/... 自定义路径：`)
            : `在 ${state.currentDisplay || "~"} 下新建目录名称，或输入 ~/... 自定义路径：`
        );
        if (!raw || !raw.trim()) return;
        const trimmed = raw.trim();
        const isFullPath = trimmed.startsWith("/") || trimmed.startsWith("~") || trimmed.includes(":\\") || trimmed.includes(":/");
        state.dirCreating = true;
        state.dirError = "";
        this._renderNewSheetSubView();
        try {
          const payload = isFullPath
            ? { path: trimmed }
            : { parentPath: state.currentPath, name: trimmed };
          const res = await fetch("/gateway/fs/mkdir", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
          });
          const d = await res.json();
          if (!res.ok || !d.ok) throw new Error(d.error || "创建本地目录失败");
          if (d.path) {
            await loadDirs(d.path);
            showToast?.(d.alreadyExists ? "已选中已有目录" : "已新建并选中目录", 2000);
          }
        } catch (err) {
          state.dirError = err.message || "创建本地目录失败";
        } finally {
          state.dirCreating = false;
          if (this._newSheetSubState === state) this._renderNewSheetSubView();
        }
      };

      const finishProject = (project, repoSlug, alreadyExists) => {
        if (state.pollTimer) {
          clearInterval(state.pollTimer);
          state.pollTimer = 0;
        }
        state.phase = "done";
        state.completedProject = project;
        state.completedRepoSlug = repoSlug || "";
        state.completedAlreadyExists = !!alreadyExists;
        this._refreshDiscoveredProjectsList(project);
        this._renderNewSheetSubView();
      };

      const startClone = async (importExisting) => {
        const parsed = this.parseGitHubRepoSlug(state.repoInput);
        if (!parsed || !state.currentPath || state.submitting) return;
        state.submitting = true;
        state.error = "";
        state.conflictAlreadyClonedPath = "";
        this._renderNewSheetSubView();
        try {
          const res = await fetch("/gateway/projects/clone", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
              repoSlug: state.repoInput.trim(),
              parentDir: state.currentPath,
              folderName: state.customFolderName.trim() || undefined,
              async: true,
              importExisting: !!importExisting,
              startSession: false,
            }),
          });
          const d = await res.json();
          if (res.status === 409 && d.code === "already_cloned") {
            state.conflictAlreadyClonedPath = d.existingPath || d.targetPath || "";
            return;
          }
          if (res.status === 409 && (d.code === "target_exists" || d.code === "path_is_file")) {
            state.showFolderRename = true;
            state.error = d.error || "目标目录已存在，请更换目录或修改文件夹名";
            return;
          }
          if (!res.ok || !d.ok) {
            throw new Error(d.error || "克隆失败");
          }
          if (d.status === "done" && d.project) {
            finishProject(d.project, d.repoSlug || parsed.slug, d.alreadyExists);
            return;
          }
          if (!d.jobId) throw new Error("网关未返回有效的克隆任务 ID");

          state.phase = "running";
          state.jobId = d.jobId;
          state.progress = d.progress || 5;
          state.progressMessage = d.message || "正在连接 GitHub 并克隆仓库…";
          state.targetPath = d.targetPath || "";
          this._renderNewSheetSubView();

          let failCount = 0;
          state.pollTimer = setInterval(async () => {
            if (this._newSheetSubState !== state) {
              clearInterval(state.pollTimer);
              return;
            }
            try {
              const stRes = await fetch(`/gateway/projects/clone/status?jobId=${encodeURIComponent(state.jobId)}`);
              const st = await stRes.json();
              if (!stRes.ok) throw new Error(st.error || "查询进度失败");
              failCount = 0;
              state.targetPath = st.targetPath || st.path || state.targetPath;
              if (st.status === "done") {
                const proj = st.project || {
                  name: (state.targetPath || parsed.repoName).split(/[\\/]/).filter(Boolean).pop(),
                  uri: st.folderUri || `file://${state.targetPath}`,
                  path: st.path || state.targetPath,
                  isWorkspace: false,
                };
                finishProject(proj, st.repoSlug || parsed.slug, st.alreadyExists);
              } else if (st.status === "failed") {
                clearInterval(state.pollTimer);
                state.pollTimer = 0;
                state.phase = "form";
                state.error = st.error || st.message || "克隆失败";
                this._renderNewSheetSubView();
              } else if (st.status === "cancelled") {
                clearInterval(state.pollTimer);
                state.pollTimer = 0;
                state.phase = "form";
                state.error = "已取消克隆";
                this._renderNewSheetSubView();
              } else {
                state.progress = Math.max(0, Math.min(100, Number(st.progress || 10)));
                state.progressMessage = st.message || "正在克隆仓库…";
                this._renderNewSheetSubView();
              }
            } catch (pollErr) {
              failCount += 1;
              if (failCount >= 6) {
                clearInterval(state.pollTimer);
                state.pollTimer = 0;
                state.phase = "form";
                state.error = "查询克隆进度失败: " + (pollErr.message || "");
                this._renderNewSheetSubView();
              }
            }
          }, 500);
        } catch (err) {
          state.error = err.message || "克隆失败";
        } finally {
          state.submitting = false;
          if (this._newSheetSubState === state) this._renderNewSheetSubView();
        }
      };

      const cancelClone = async () => {
        const jid = state.jobId;
        if (state.pollTimer) {
          clearInterval(state.pollTimer);
          state.pollTimer = 0;
        }
        state.phase = "form";
        state.error = "已取消克隆";
        this._renderNewSheetSubView();
        if (jid) {
          fetch("/gateway/projects/clone/cancel", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ jobId: jid }),
          }).catch(() => {});
        }
      };

      const addWorkspace = async (overridePath) => {
        const target = (overridePath || state.currentPath || "").trim();
        if (!target || state.submitting) return;
        state.submitting = true;
        state.error = "";
        this._renderNewSheetSubView();
        try {
          const res = await fetch("/gateway/projects/create", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ path: target, startSession: false }),
          });
          const d = await res.json();
          if (!res.ok || !d.ok) throw new Error(d.error || "添加工作区失败");
          const proj = d.project || {
            name: target.split(/[\\/]/).filter(Boolean).pop() || target,
            uri: target.startsWith("file://") ? target : `file://${target}`,
            path: target,
            isWorkspace: false,
          };
          finishProject(proj, "", d.alreadyExists);
        } catch (err) {
          state.error = err.message || "添加工作区失败";
        } finally {
          state.submitting = false;
          if (this._newSheetSubState === state) this._renderNewSheetSubView();
        }
      };

      state.actions = { loadDirs, createDir, startClone, cancelClone, addWorkspace };
      loadDirs("");
    },

    _renderNewSheetSubView() {
      const subEl = document.getElementById("sheet-new-sub");
      const st = this._newSheetSubState;
      if (!subEl || !st) return;

      const isClone = st.mode === "clone";
      const parsed = isClone ? this.parseGitHubRepoSlug(st.repoInput) : null;
      const effFolder = isClone ? (st.customFolderName.trim() || (parsed ? parsed.repoName : "")) : "";
      const sep = (st.currentPath || "").includes("\\") ? "\\" : "/";
      const previewBase = st.currentDisplay || st.currentPath || "";
      const previewTarget = isClone
        ? (previewBase && effFolder ? (previewBase.endsWith(sep) ? previewBase + effFolder : previewBase + sep + effFolder) : previewBase)
        : st.currentPath;

      const headerTitle = isClone ? "从 GitHub 克隆仓库" : "添加本地工作区";
      const backBtnHtml = st.phase === "running"
        ? `<div style="width:52px;"></div>`
        : `<button type="button" class="sheet-header-btn" data-sub-action="back">‹ 返回</button>`;
      const rightBtnHtml = !isClone && st.phase === "form"
        ? `<button type="button" class="sheet-header-btn" data-sub-action="manual-path" style="font-size:13.5px;">手输路径</button>`
        : `<div style="width:52px;"></div>`;

      let bodyHtml = "";
      if (st.phase === "running") {
        bodyHtml = `
          <div class="clone-center-state">
            <div style="font-size:17px;font-weight:700;color:var(--ios-label);">${escapeHtml(parsed?.slug || st.repoInput)}</div>
            <div style="font-size:13.5px;color:var(--ios-secondary-label);">${escapeHtml(st.progressMessage || "正在克隆仓库…")}</div>
            <div style="width:100%;background:var(--ios-tertiary-bg);padding:14px;border-radius:14px;display:flex;flex-direction:column;gap:8px;">
              <div class="clone-progress-track"><div class="clone-progress-fill" style="width:${st.progress}%;"></div></div>
              <div style="display:flex;justify-content:space-between;gap:8px;font-size:11.5px;">
                <span class="monospaced" style="color:var(--ios-secondary-label);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">${escapeHtml(st.targetPath)}</span>
                <span class="monospaced" style="color:var(--ios-blue);font-weight:600;">${st.progress}%</span>
              </div>
            </div>
            <button type="button" class="clone-cancel-btn" data-sub-action="cancel-clone">取消克隆</button>
          </div>
        `;
      } else if (st.phase === "done" && st.completedProject) {
        const proj = st.completedProject;
        const doneTitle = isClone
          ? (st.completedAlreadyExists ? "已导入 Antigravity 项目列表" : "克隆完成，已加入项目列表")
          : (st.completedAlreadyExists ? "工作区已在项目列表中" : "已添加到 Antigravity 项目列表");
        bodyHtml = `
          <div class="clone-center-state">
            <div style="width:62px;height:62px;border-radius:999px;background:rgba(48,209,88,0.15);color:var(--ios-green);display:flex;align-items:center;justify-content:center;font-size:30px;font-weight:700;">✓</div>
            <div style="font-size:18px;font-weight:700;color:var(--ios-label);">${doneTitle}</div>
            <div style="font-size:13px;color:var(--ios-secondary-label);">现在可以在「新建会话」工作区列表中随时选用该项目</div>
            <div style="width:100%;text-align:left;background:var(--ios-tertiary-bg);padding:14px;border-radius:14px;display:flex;flex-direction:column;gap:6px;">
              <div style="font-size:15.5px;font-weight:600;color:var(--ios-label);">${escapeHtml(proj.alias || proj.name)}</div>
              ${st.completedRepoSlug ? `<div class="monospaced" style="font-size:12px;color:var(--ios-secondary-label);">${escapeHtml(st.completedRepoSlug)}</div>` : ""}
              <div class="monospaced" style="font-size:11.5px;color:var(--ios-secondary-label);word-break:break-all;padding-top:4px;border-top:0.5px solid var(--ios-separator-subtle);">${escapeHtml(proj.path || "")}</div>
            </div>
          </div>
          <div class="dir-sub-footer" style="display:flex;flex-direction:column;gap:10px;">
            <button type="button" class="dir-primary-btn" data-sub-action="start-chat">立即开启新会话</button>
            <button type="button" class="dir-secondary-btn" data-sub-action="back">返回工作区列表</button>
          </div>
        `;
      } else {
        const chipsHtml = st.recommendedDirs.length > 0
          ? `<div class="dir-chips-row">${st.recommendedDirs.map((d) => `
              <button type="button" class="dir-chip ${d.path === st.currentPath ? "active" : ""}" data-dir-jump="${escapeHtml(d.path)}">
                <span>${escapeHtml(d.name)}</span>
                ${d.projectCount > 0 ? `<span class="dir-chip-count">${d.projectCount}</span>` : ""}
              </button>
            `).join("")}</div>`
          : "";

        let entriesHtml = "";
        if (st.dirError) {
          entriesHtml = `<div style="padding:14px;font-size:12.5px;color:var(--ios-orange);">${escapeHtml(st.dirError)}</div>`;
        } else if (st.dirLoading && st.entries.length === 0) {
          entriesHtml = `<div style="padding:20px;text-align:center;font-size:12.5px;color:var(--ios-secondary-label);">正在读取本机目录…</div>`;
        } else if (st.entries.length === 0) {
          entriesHtml = `<div style="padding:20px;text-align:center;font-size:12.5px;color:var(--ios-secondary-label);">当前目录下暂无子文件夹，可直接选用此目录或点击「+ 新建目录」</div>`;
        } else {
          entriesHtml = `<div class="dir-entries-list">${st.entries.map((e) => `
            <div class="dir-entry-row" data-dir-enter="${escapeHtml(e.path)}">
              <span>${e.isProject ? "💼" : "📁"}</span>
              <span class="dir-entry-name">${escapeHtml(e.name)}</span>
              ${e.isProject ? `<span class="dir-tag project">项目</span>` : (e.isGitRepo ? `<span class="dir-tag git">Git</span>` : "")}
              <span style="color:var(--ios-tertiary-label);font-size:13px;">›</span>
            </div>
          `).join("")}</div>`;
        }

        const repoSectionHtml = isClone ? `
          <div style="display:flex;flex-direction:column;gap:8px;">
            <div class="dir-section-title">GitHub 仓库地址</div>
            <input id="clone-repo-input" class="dir-input ${parsed ? "is-valid" : ""}" type="text" placeholder="owner/repo 或 https://github.com/..." value="${escapeHtml(st.repoInput)}" autocomplete="off" spellcheck="false" />
            <div style="display:flex;justify-content:space-between;align-items:center;gap:8px;font-size:12px;">
              ${parsed
                ? `<span style="color:var(--ios-green);font-weight:500;">✓ ${escapeHtml(parsed.slug)}</span>
                   <button type="button" style="background:none;border:none;color:var(--ios-blue);font-size:12px;cursor:pointer;padding:0;" data-sub-action="toggle-rename">${st.showFolderRename ? "收起目录名" : "自定义目录名"}</button>`
                : (st.repoInput.trim()
                    ? `<span style="color:var(--ios-orange);">支持 owner/repo、HTTPS 或 git@github.com 地址</span>`
                    : `<span style="color:var(--ios-secondary-label);">克隆完成后会自动加入 Antigravity 项目列表</span>`)}
            </div>
            ${st.showFolderRename ? `<input id="clone-folder-input" class="dir-input" type="text" placeholder="文件夹名（默认 ${escapeHtml(parsed?.repoName || "仓库名")}）" value="${escapeHtml(st.customFolderName)}" autocomplete="off" spellcheck="false" />` : ""}
          </div>
        ` : "";

        const conflictOrErrHtml = st.conflictAlreadyClonedPath
          ? `<div class="dir-alert-card">
               <div style="font-weight:600;">该目录已存在同一 GitHub 仓库</div>
               <div class="monospaced" style="font-size:11.5px;color:var(--ios-secondary-label);word-break:break-all;">${escapeHtml(st.conflictAlreadyClonedPath)}</div>
               <button type="button" class="dir-warn-btn" data-sub-action="import-existing" ${st.submitting ? "disabled" : ""}>直接导入到 Antigravity 项目列表</button>
             </div>`
          : (st.error ? `<div class="dir-alert-card">${escapeHtml(st.error)}</div>` : "");

        const canSubmit = isClone
          ? (!!parsed && !!st.currentPath && !!effFolder && !st.submitting)
          : (!!st.currentPath && !st.submitting);

        bodyHtml = `
          <div class="dir-sub-body">
            ${repoSectionHtml}
            <div style="display:flex;flex-direction:column;gap:10px;">
              <div class="dir-section-head">
                <div>
                  <div class="dir-section-title">${isClone ? "选择本机存放目录" : "浏览或新建本机文件夹"}</div>
                  <div class="dir-section-sub">${isClone ? "可从已有项目目录选择，浏览子目录，或新建本地目录" : "进入目标目录后点击底部按钮，即可将其加入项目列表"}</div>
                </div>
                <button type="button" class="dir-pill-btn" data-sub-action="mkdir" ${st.dirCreating || !st.currentPath ? "disabled" : ""}>${st.dirCreating ? "创建中…" : "+ 新建目录"}</button>
              </div>
              ${chipsHtml}
              <div class="dir-browser-box">
                <div class="dir-browser-bar">
                  ${st.canGoUp ? `<button type="button" class="dir-up-btn" data-dir-jump="${escapeHtml(st.parentPath)}">‹ 上一级</button>` : ""}
                  <span class="dir-current-path" title="${escapeHtml(st.currentPath)}">${escapeHtml(st.currentDisplay || "加载中…")}</span>
                </div>
                ${entriesHtml}
              </div>
            </div>
            ${previewTarget ? `
              <div class="dir-preview-card">
                <div class="dir-preview-label">${isClone ? "克隆目标路径" : "当前选定目录"}</div>
                <div class="dir-preview-path">${escapeHtml(previewTarget)}</div>
              </div>
            ` : ""}
            ${conflictOrErrHtml}
          </div>
          <div class="dir-sub-footer">
            <button type="button" class="dir-primary-btn" data-sub-action="submit" ${canSubmit ? "" : "disabled"}>
              ${st.submitting
                ? (isClone ? "正在启动克隆…" : "正在添加工作区…")
                : (isClone ? "确认并克隆" : "添加当前目录为工作区")}
            </button>
          </div>
        `;
      }

      subEl.innerHTML = `
        <div class="sheet-header">
          ${backBtnHtml}
          <h2 class="sheet-title">${headerTitle}</h2>
          ${rightBtnHtml}
        </div>
        ${bodyHtml}
      `;

      // Bind inputs & actions without losing focus on typing
      const repoInputEl = subEl.querySelector("#clone-repo-input");
      if (repoInputEl) {
        repoInputEl.addEventListener("input", (e) => {
          const val = e.target.value;
          const pos = e.target.selectionStart;
          st.repoInput = val;
          st.error = "";
          st.conflictAlreadyClonedPath = "";
          this._renderNewSheetSubView();
          const refreshed = document.querySelector("#sheet-new-sub #clone-repo-input");
          if (refreshed) {
            refreshed.focus();
            if (typeof pos === "number") refreshed.setSelectionRange(pos, pos);
          }
        });
      }
      const folderInputEl = subEl.querySelector("#clone-folder-input");
      if (folderInputEl) {
        folderInputEl.addEventListener("input", (e) => {
          const val = e.target.value;
          const pos = e.target.selectionStart;
          st.customFolderName = val;
          st.error = "";
          st.conflictAlreadyClonedPath = "";
          this._renderNewSheetSubView();
          const refreshed = document.querySelector("#sheet-new-sub #clone-folder-input");
          if (refreshed) {
            refreshed.focus();
            if (typeof pos === "number") refreshed.setSelectionRange(pos, pos);
          }
        });
      }

      subEl.querySelectorAll("[data-dir-jump]").forEach((btn) => {
        btn.addEventListener("click", () => st.actions.loadDirs(btn.getAttribute("data-dir-jump")));
      });
      subEl.querySelectorAll("[data-dir-enter]").forEach((row) => {
        row.addEventListener("click", () => st.actions.loadDirs(row.getAttribute("data-dir-enter")));
      });
      subEl.querySelectorAll("[data-sub-action]").forEach((btn) => {
        btn.addEventListener("click", () => {
          const act = btn.getAttribute("data-sub-action");
          if (act === "back") {
            this.resetNewSheetSubView();
          } else if (act === "mkdir") {
            st.actions.createDir();
          } else if (act === "toggle-rename") {
            st.showFolderRename = !st.showFolderRename;
            this._renderNewSheetSubView();
          } else if (act === "submit") {
            if (isClone) st.actions.startClone(false);
            else st.actions.addWorkspace("");
          } else if (act === "import-existing") {
            st.actions.startClone(true);
          } else if (act === "cancel-clone") {
            st.actions.cancelClone();
          } else if (act === "manual-path") {
            const p = prompt(typeof t === "function" ? t("输入电脑本地目录路径 (如 ~/Projects/my-app)：") : "输入电脑本地目录路径 (如 ~/Projects/my-app)：", st.currentPath || "");
            if (p && p.trim()) st.actions.addWorkspace(p.trim());
          } else if (act === "start-chat" && st.completedProject) {
            const proj = st.completedProject;
            closeNewSheet?.();
            startDraftSession?.({
              isPure: false,
              name: proj.alias || proj.name,
              path: proj.path,
              uri: proj.uri || proj.path,
              rawId: proj.rawId || (proj.id !== proj.path ? proj.id : undefined),
            });
          }
        });
      });
    },

    bindNewSheetExtras() {
      const cloneBtn = document.getElementById("btn-clone-repo");
      cloneBtn?.addEventListener("click", () => {
        this._openNewSheetSubView("clone");
      });

      const addWorkspaceBtn = document.getElementById("btn-add-workspace");
      addWorkspaceBtn?.addEventListener("click", () => {
        this._openNewSheetSubView("add");
      });

      const listEl = document.getElementById("new-projects-list");
      if (!listEl) return;
      let lpTimer = 0;
      let suppressNextClick = false;

      const showProjectMenu = (card) => {
        const idx = Number(card.getAttribute("data-index"));
        const proj = Array.isArray(discoveredProjects) ? discoveredProjects[idx] : null;
        if (!proj) return;
        const wsMenuPrompt = window.I18n?.isZh?.()
          ? `工作区「${proj.name}」操作：\n1 = 在独立 Git Worktree 开启新会话\n2 = 设置工作区备注别名\n请输入序号 (1 或 2)：`
          : (window.I18n?.isJa?.()
              ? `ワークスペース「${proj.name}」の操作：\n1 = 独立した Git Worktree で新規会話を開始\n2 = ワークスペースのメモ・別名を設定\n番号を入力 (1 または 2)：`
              : `Workspace "${proj.name}" actions:\n1 = Start new chat in an isolated Git Worktree\n2 = Set workspace alias / note\nEnter choice (1 or 2):`);
        const action = prompt(wsMenuPrompt);
        if (action === "1") {
          this.createWorktreeSession(proj);
        } else if (action === "2") {
          const cur = this.workspaceNotes[proj.uri || proj.path] || "";
          const note = prompt(typeof t === "function" ? t(`为「${proj.name}」设置备注 (留空清除)：`) : `为「${proj.name}」设置备注 (留空清除)：`, cur);
          if (note !== null) {
            if (note.trim()) this.workspaceNotes[proj.uri || proj.path] = note.trim();
            else delete this.workspaceNotes[proj.uri || proj.path];
            localStorage.setItem("agy_workspace_notes", JSON.stringify(this.workspaceNotes));
            renderNewProjectsList?.(discoveredProjects);
          }
        }
      };

      listEl.addEventListener("touchstart", (e) => {
        const card = e.target.closest(".project-select-card[data-index]");
        if (!card) return;
        suppressNextClick = false;
        lpTimer = setTimeout(() => {
          suppressNextClick = true;
          triggerHaptic?.("medium");
          showProjectMenu(card);
        }, 480);
      }, { passive: true });
      listEl.addEventListener("touchend", () => clearTimeout(lpTimer));
      listEl.addEventListener("touchmove", () => clearTimeout(lpTimer), { passive: true });
      listEl.addEventListener("contextmenu", (e) => {
        const card = e.target.closest(".project-select-card[data-index]");
        if (!card) return;
        e.preventDefault();
        showProjectMenu(card);
      });
      listEl.addEventListener("click", (e) => {
        if (suppressNextClick) {
          suppressNextClick = false;
          e.stopImmediatePropagation();
        }
      }, true);
    },

    async createWorktreeSession(proj) {
      showToast?.(`正在为「${proj.name}」创建独立 Worktree…`, 5000);
      try {
        const res = await fetch("/gateway/worktree/create", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            repoUri: proj.uri,
            workspacePath: proj.path,
            startSession: true,
          }),
        });
        const d = await res.json();
        if (!res.ok || !d.ok) throw new Error(d.error || "创建失败");
        document.getElementById("sheet-new")?.classList.add("hidden");
        if (d.cascadeId) navigateTo?.("#c=" + d.cascadeId);
        else showToast?.("已创建 Worktree: " + (d.worktreeName || ""));
      } catch (err) {
        showToast?.("Worktree 创建失败: " + err.message, 3200);
      }
    },

    // --- 4. `@` Context Mentions ---
    bindMentionTriggers() {
      const inputEl = document.getElementById("chat-input");
      if (inputEl) {
        inputEl.addEventListener("input", () => {
          clearTimeout(this._mentionDebounceTimer);
          const val = inputEl.value;
          const m = val.match(/(?:^|\s)@([^\s@]*)$/);
          if (m) {
            const q = m[1];
            this._mentionDebounceTimer = setTimeout(() => {
              this.openMentionPicker(q);
            }, 150);
          } else if (this.mentionOpen) {
            this.closeMentionPicker();
          }
        });
      }

      const addBtn = document.getElementById("btn-add-image");
      if (addBtn) {
        let lpTimer = 0;
        let held = false;
        addBtn.addEventListener("touchstart", () => {
          held = false;
          lpTimer = setTimeout(() => {
            held = true;
            triggerHaptic?.("light");
            this.openMentionPicker("");
          }, 420);
        }, { passive: true });
        addBtn.addEventListener("touchend", () => clearTimeout(lpTimer));
        addBtn.addEventListener("contextmenu", (e) => {
          e.preventDefault();
          this.openMentionPicker("");
        });
        addBtn.addEventListener("click", (e) => {
          if (held) {
            held = false;
            e.stopImmediatePropagation();
          }
        }, true);
      }
    },

    async openMentionPicker(query) {
      const reqSeq = (this._mentionReqSeq = (this._mentionReqSeq || 0) + 1);
      let picker = document.getElementById("context-mention-picker");
      if (!picker) {
        picker = document.createElement("div");
        picker.id = "context-mention-picker";
        picker.className = "p3-mention-picker hidden";
        const bar = document.getElementById("input-bar-wrapper") || document.querySelector(".chat-input-bar");
        if (bar) bar.parentElement?.insertBefore(picker, bar);
        else document.body.appendChild(picker);
      }
      this.mentionOpen = true;
      picker.classList.remove("hidden");
      picker.innerHTML = `<div class="p3-mention-head"><span>@ 引用文件 / 目录 / 历史会话</span><button type="button" id="btn-mention-close" class="p3-mini-btn">关闭</button></div><div class="p3-mention-list"><div class="p3-empty">搜索中…</div></div>`;
      document.getElementById("btn-mention-close")?.addEventListener("click", () => this.closeMentionPicker());

      const wsUri = (typeof currentTrajectories === "object" && activeCascadeId && currentTrajectories[activeCascadeId]?.workspaceUris?.[0]) || "";
      try {
        const res = await fetch("/gateway/context/mentions", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ query: query || "", cascadeId: activeCascadeId || "", workspaceUri: wsUri, limit: 25 }),
        });
        const data = await res.json();
        if (reqSeq !== this._mentionReqSeq || !this.mentionOpen) return;
        const items = Array.isArray(data.items) ? data.items : [];
        this.mentionItems = items;
        const list = picker.querySelector(".p3-mention-list");
        if (!list) return;
        list.innerHTML = items.map((it, i) => `
          <button type="button" class="p3-mention-item" data-mention-idx="${i}">
            <span class="p3-mention-icon">${it.type === "directory" ? "📁" : it.type === "conversation" ? "💬" : "📄"}</span>
            <span class="p3-mention-text">
              <span class="p3-mention-label">${escapeHtml(it.label)}</span>
              ${it.detail ? `<span class="p3-mention-detail">${escapeHtml(it.detail)}</span>` : ""}
            </span>
          </button>`).join("") || `<div class="p3-empty">未找到匹配上下文</div>`;
        list.querySelectorAll("[data-mention-idx]").forEach((b) => {
          b.addEventListener("click", () => {
            const item = this.mentionItems[Number(b.getAttribute("data-mention-idx"))];
            if (item) this.selectMention(item);
          });
        });
      } catch (_) {}
    },

    selectMention(item) {
      const inputEl = document.getElementById("chat-input");
      if (inputEl) {
        inputEl.value = inputEl.value.replace(/(?:^|\s)@[^\s@]*$/, (m) => (m.startsWith(" ") ? " " : ""));
        inputEl.focus();
      }
      if (!this.contextItems.some((x) => x.id === item.id)) {
        this.contextItems.push(item);
      }
      this.closeMentionPicker();
      this.renderContextChips();
    },

    closeMentionPicker() {
      clearTimeout(this._mentionDebounceTimer);
      this.mentionOpen = false;
      document.getElementById("context-mention-picker")?.classList.add("hidden");
    },

    renderContextChips() {
      const bar = document.getElementById("context-chips-bar");
      if (!bar) return;
      if (this.contextItems.length === 0) {
        bar.classList.add("hidden");
        bar.innerHTML = "";
        this.renderSuggestedResponses();
        return;
      }
      bar.classList.remove("hidden");
      bar.innerHTML = this.contextItems.map((it, idx) => `
        <span class="p3-context-chip">
          <span>${it.type === "directory" ? "📁" : it.type === "conversation" ? "💬" : "📄"} ${escapeHtml(it.label)}</span>
          <button type="button" data-ctx-rm="${idx}" aria-label="移除">✕</button>
        </span>`).join("");
      bar.querySelectorAll("[data-ctx-rm]").forEach((b) => {
        b.addEventListener("click", () => {
          this.contextItems.splice(Number(b.getAttribute("data-ctx-rm")), 1);
          this.renderContextChips();
        });
      });
      this.renderSuggestedResponses();
    },

    consumeContextItems() {
      const items = [...this.contextItems];
      this.contextItems = [];
      this.renderContextChips();
      return items;
    },

    // --- 5. Suggested Responses (hides when input focused, drafting, or dismissed) ---
    syncStreamData(data) {
      if (!data) return;
      const isRunning = data.status === "CASCADE_RUN_STATUS_RUNNING";
      const incoming = !isRunning && Array.isArray(data.suggestedResponses) ? data.suggestedResponses : [];
      const incomingSource = incoming.length > 0 ? (data.suggestedResponseSource || "native") : "";
      if (this.dismissedPredictedSuggestion && incoming[0] !== this.dismissedPredictedSuggestion) {
        this.dismissedPredictedSuggestion = null;
      }
      this.suggestedResponses = incoming;
      this.suggestedResponseSource = incomingSource;
      this.renderSuggestedResponses();
    },

    bindSuggestedResponsesVisibility() {
      const inputEl = document.getElementById("chat-input");
      if (inputEl) {
        inputEl.addEventListener("input", () => this.renderSuggestedResponses());
      }
      window.addEventListener("resize", () => this.syncSuggestedChipMaxWidth());
    },

    syncSuggestedChipMaxWidth() {
      const bar = document.getElementById("suggested-responses-bar");
      const qaBar = document.getElementById("quick-actions-bar");
      const addBtn = document.getElementById("btn-add-image");
      if (!bar || !qaBar || !addBtn) return;
      const barW = qaBar.clientWidth || 0;
      if (barW <= 0) return;
      const cs = window.getComputedStyle(qaBar);
      const padL = parseFloat(cs.paddingLeft) || 0;
      const padR = parseFloat(cs.paddingRight) || 0;
      const addW = addBtn.offsetWidth || 34;
      // 8px gap after +, 6px right clearance for dismiss badge; extends past model pill before wrapping
      const maxW = Math.max(180, Math.floor(barW - padL - padR - addW - 14));
      bar.style.setProperty("--suggest-max-w", `${maxW}px`);
    },

    renderSuggestedResponses() {
      const bar = document.getElementById("suggested-responses-bar");
      if (!bar) return;
      this.syncSuggestedChipMaxWidth();
      const inputEl = document.getElementById("chat-input");
      const hasDraft = Boolean(inputEl && inputEl.value.trim().length > 0);
      const hasStagedDraft = Boolean(
        (typeof pendingImages !== "undefined" && Array.isArray(pendingImages) && pendingImages.length > 0) ||
        (typeof pendingAttachments !== "undefined" && Array.isArray(pendingAttachments) && pendingAttachments.length > 0) ||
        (typeof getSelectedSlashCommands === "function" && getSelectedSlashCommands().length > 0) ||
        (Array.isArray(this.contextItems) && this.contextItems.length > 0)
      );
      const interactionEl = document.getElementById("interaction-card-container");
      const hasPendingInteraction = Boolean(
        (typeof currentPendingInteraction !== "undefined" && currentPendingInteraction) ||
        (interactionEl && !interactionEl.classList.contains("hidden") && interactionEl.childElementCount > 0)
      );
      const proceedEl = document.getElementById("btn-proceed");
      const continueEl = document.getElementById("btn-continue");
      const hasPrimaryAction = Boolean(
        (typeof currentCanProceed !== "undefined" && currentCanProceed) ||
        (proceedEl && !proceedEl.classList.contains("hidden")) ||
        (continueEl && !continueEl.classList.contains("hidden"))
      );
      const isRunning = typeof isCascadeRunning === "function" && isCascadeRunning(activeCascadeId);
      const isPredicted = this.suggestedResponseSource === "predicted";
      const visible = (!hasDraft && !hasStagedDraft && !hasPendingInteraction && !hasPrimaryAction && !isRunning)
        ? this.suggestedResponses.filter((text) => text && text.trim().length > 0 && text !== this.dismissedPredictedSuggestion)
        : [];
      if (visible.length === 0) {
        bar.classList.add("hidden");
        bar.innerHTML = "";
        return;
      }
      bar.classList.remove("hidden");
      const sparkleSvg = isPredicted
        ? `<svg class="p3-suggest-sparkle" width="12" height="12" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 2l2.2 6.8L21 11l-6.8 2.2L12 20l-2.2-6.8L3 11l6.8-2.2L12 2z"/></svg>`
        : "";
      const dismissAria = typeof t === "function" ? t("关闭建议") : "关闭建议";
      bar.innerHTML = visible.map((text, idx) => `
        <div class="suggest-chip${isPredicted ? " is-predicted" : ""}">
          <div class="suggest-chip-progress-wrap" aria-hidden="true"><div class="suggest-chip-progress-fill"></div></div>
          <button type="button" class="suggest-chip-main" data-i18n-skip="1" data-suggest-idx="${idx}" title="${escapeHtml(text)}">${sparkleSvg}<span>${escapeHtml(text)}</span></button>
          <button type="button" class="suggest-chip-clear" data-suggest-dismiss="${idx}" aria-label="${escapeHtml(dismissAria)}">&times;</button>
        </div>`).join("");

      const HOLD_GRACE_MS = 180;
      const HOLD_FILL_MS = 1320; // 180 + 1320 = 1500ms (1.5s)
      const MOVE_SLOP_PX = 10;

      const fillSuggestion = (text) => {
        if (!text || !inputEl) return;
        this.dismissedPredictedSuggestion = text;
        inputEl.value = text;
        inputEl.dispatchEvent(new Event("input", { bubbles: true }));
        inputEl.focus();
        this.renderSuggestedResponses();
      };

      const sendSuggestionDirectly = (text) => {
        if (!text || !inputEl) return;
        inputEl.value = text;
        this.suggestedResponses = [];
        this.suggestedResponseSource = "";
        this.dismissedPredictedSuggestion = null;
        this.renderSuggestedResponses();
        sendMessage?.();
      };

      bar.querySelectorAll("[data-suggest-idx]").forEach((btn) => {
        const chipEl = btn.closest(".suggest-chip");
        const fillEl = chipEl?.querySelector(".suggest-chip-progress-fill");
        const text = visible[Number(btn.getAttribute("data-suggest-idx"))];
        let activePointerId = null;
        let startX = 0;
        let startY = 0;
        let graceTimer = 0;
        let sendTimer = 0;
        let engaged = false;
        let completed = false;
        let moved = false;

        const resetProgress = (animated) => {
          if (graceTimer) { clearTimeout(graceTimer); graceTimer = 0; }
          if (sendTimer) { clearTimeout(sendTimer); sendTimer = 0; }
          chipEl?.classList.remove("is-holding");
          if (fillEl) {
            fillEl.style.transition = animated ? "transform 160ms ease-out" : "none";
            fillEl.style.transform = "scaleX(0)";
          }
        };

        btn.addEventListener("contextmenu", (e) => e.preventDefault());

        btn.addEventListener("pointerdown", (e) => {
          if (e.pointerType === "mouse" && e.button !== 0) return;
          activePointerId = e.pointerId;
          startX = e.clientX;
          startY = e.clientY;
          engaged = false;
          completed = false;
          moved = false;
          resetProgress(false);

          graceTimer = setTimeout(() => {
            graceTimer = 0;
            if (activePointerId === null || moved || completed) return;
            engaged = true;
            chipEl?.classList.add("is-holding");
            if (fillEl) {
              fillEl.style.transition = "none";
              fillEl.style.transform = "scaleX(0)";
              void fillEl.offsetWidth;
              fillEl.style.transition = `transform ${HOLD_FILL_MS}ms linear`;
              fillEl.style.transform = "scaleX(1)";
            }
            sendTimer = setTimeout(() => {
              sendTimer = 0;
              if (activePointerId === null || moved) return;
              completed = true;
              activePointerId = null;
              chipEl?.classList.remove("is-holding");
              try { navigator.vibrate?.(20); } catch (_) {}
              sendSuggestionDirectly(text);
            }, HOLD_FILL_MS);
          }, HOLD_GRACE_MS);
        });

        btn.addEventListener("pointermove", (e) => {
          if (activePointerId !== e.pointerId) return;
          const dx = e.clientX - startX;
          const dy = e.clientY - startY;
          if (Math.hypot(dx, dy) > MOVE_SLOP_PX) {
            moved = true;
            activePointerId = null;
            resetProgress(true);
          }
        });

        const endPointer = (e, isCancel) => {
          if (activePointerId !== e.pointerId) return;
          activePointerId = null;
          if (completed) return;
          const wasEngaged = engaged;
          const wasMoved = moved;
          resetProgress(wasEngaged);
          if (!isCancel && !wasMoved && !wasEngaged) {
            fillSuggestion(text);
          }
        };

        btn.addEventListener("pointerup", (e) => endPointer(e, false));
        btn.addEventListener("pointercancel", (e) => endPointer(e, true));
        btn.addEventListener("pointerleave", (e) => {
          if (e.pointerType === "mouse") endPointer(e, true);
        });
        btn.addEventListener("keydown", (e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            fillSuggestion(text);
          }
        });
      });
      bar.querySelectorAll("[data-suggest-dismiss]").forEach((btn) => {
        btn.addEventListener("click", (e) => {
          e.stopPropagation();
          const idx = Number(btn.getAttribute("data-suggest-dismiss"));
          const chosen = visible[idx] || visible[0] || null;
          this.dismissedPredictedSuggestion = chosen;
          if (chosen && this.suggestedResponses.length > 1) {
            this.suggestedResponses = this.suggestedResponses.filter((s) => s !== chosen);
          }
          this.renderSuggestedResponses();
        });
      });
    },

    // --- 6. Long-Press Right Voice Button -> Streaming Dictation ---
    bindDictationLongPress() {
      const btn = document.getElementById("btn-send");
      if (!btn) return;
      let timer = 0;
      let holding = false;

      const canDictate = () => {
        const inputEl = document.getElementById("chat-input");
        const hasText = inputEl && inputEl.value.trim().length > 0;
        const isRunning = typeof isCascadeRunning === "function" && isCascadeRunning(activeCascadeId);
        return !hasText && !isRunning;
      };

      btn.addEventListener("touchstart", () => {
        if (!canDictate()) return;
        holding = false;
        timer = setTimeout(() => {
          holding = true;
          this.startDictation();
        }, 450);
      }, { passive: true });
      btn.addEventListener("touchend", () => {
        clearTimeout(timer);
        if (holding) {
          holding = false;
          this.stopDictation();
        }
      });
      btn.addEventListener("touchcancel", () => {
        clearTimeout(timer);
        if (holding) {
          holding = false;
          this.stopDictation();
        }
      });
    },

    async startDictation() {
      if (this.isDictating) return;
      this.isDictating = true;
      triggerHaptic?.("medium");
      showToast?.("🎙️ 正在聆听实时听写（松开结束）…", 10000);
      const btn = document.getElementById("btn-send");
      btn?.classList.add("dictating");

      try {
        const stream = await navigator.mediaDevices.getUserMedia({ audio: { channelCount: 1, sampleRate: 16000 } });
        if (!this.isDictating) {
          stream.getTracks().forEach((t) => t.stop());
          return;
        }
        this.dictateStream = stream;
        const ticketParam = await this.acquireWsTicketQueryParam("&");
        if (!this.isDictating) {
          stream.getTracks().forEach((t) => t.stop());
          this.dictateStream = null;
          return;
        }
        const proto = location.protocol === "https:" ? "wss:" : "ws:";
        const ws = new WebSocket(`${proto}//${location.host}/gateway/audio/dictate?cascadeId=${encodeURIComponent(activeCascadeId || "")}&mimeType=${encodeURIComponent("audio/pcm;rate=16000")}${ticketParam}`);
        this.dictateWs = ws;

        const inputEl = document.getElementById("chat-input");
        const baseText = inputEl ? inputEl.value : "";

        ws.onmessage = (ev) => {
          try {
            const msg = JSON.parse(ev.data);
            const txt = msg.text || msg.transcript || msg.transcription?.text || msg.update?.text || msg.update?.transcript || "";
            if (txt && inputEl) {
              inputEl.value = baseText ? (baseText.trimEnd() + " " + txt) : txt;
              inputEl.dispatchEvent(new Event("input", { bubbles: true }));
            }
          } catch (_) {}
        };

        const AC = window.AudioContext || window.webkitAudioContext;
        const ac = new AC({ sampleRate: 16000 });
        this.dictateCtx = ac;
        const src = ac.createMediaStreamSource(stream);
        const proc = ac.createScriptProcessor(4096, 1, 1);
        proc.onaudioprocess = (e) => {
          if (!this.isDictating || ws.readyState !== 1) return;
          const f32 = e.inputBuffer.getChannelData(0);
          const i16 = new Int16Array(f32.length);
          for (let i = 0; i < f32.length; i++) {
            const s = Math.max(-1, Math.min(1, f32[i]));
            i16[i] = s < 0 ? s * 0x8000 : s * 0x7fff;
          }
          ws.send(i16.buffer);
        };
        src.connect(proc);
        proc.connect(ac.destination);
      } catch (err) {
        showToast?.("麦克风或听写初始化失败: " + err.message, 2500);
        this.stopDictation();
      }
    },

    stopDictation() {
      if (!this.isDictating) return;
      this.isDictating = false;
      document.getElementById("btn-send")?.classList.remove("dictating");
      showToast?.("听写结束，可编辑后发送", 1200);
      if (this.dictateWs && this.dictateWs.readyState === 1) {
        try { this.dictateWs.send(JSON.stringify({ type: "end" })); } catch (_) {}
      }
      setTimeout(() => {
        try { this.dictateWs?.close(); } catch (_) {}
        this.dictateWs = null;
      }, 400);
      try { this.dictateCtx?.close(); } catch (_) {}
      this.dictateCtx = null;
      this.dictateStream?.getTracks()?.forEach((t) => t.stop());
      this.dictateStream = null;
    },

    // --- 7. Fork & Narration ---
    async forkAtStep(stepIndex) {
      if (!activeCascadeId) return;
      const useWt = confirm(typeof t === "function" ? t("从该步骤分叉新会话？\n点击「确定」将在独立 Git Worktree 创建分叉（失败可取消改普通分叉）；或请用 Worktree 参数。") : "从该步骤分叉新会话？\n点击「确定」将在独立 Git Worktree 创建分叉（失败可取消改普通分叉）；或请用 Worktree 参数。");
      try {
        const res = await fetch("/gateway/cascade/fork", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            cascadeId: activeCascadeId,
            stepIndex: Number(stepIndex) || 0,
            createWorktree: !!useWt,
          }),
        });
        const d = await res.json();
        if (!res.ok || !d.ok || !d.cascadeId) throw new Error(d.error || "分叉失败");
        showToast?.("已分叉新会话");
        navigateTo?.("#c=" + d.cascadeId);
      } catch (err) {
        showToast?.("分叉失败: " + err.message, 3000);
      }
    },

    async narrateText(text) {
      const cleaned = String(text || "").trim();
      if (!cleaned) return;
      this.stopNarration();
      const bar = document.getElementById("narration-bar");
      const title = document.getElementById("narration-title");
      bar?.classList.remove("hidden");
      if (title) title.textContent = "🔊 正在合成语音…";

      try {
        const res = await fetch("/gateway/narration", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ text: cleaned }),
        });
        if (!res.ok) throw new Error("HTTP " + res.status);
        const blob = await res.blob();
        const url = URL.createObjectURL(blob);
        const audio = new Audio(url);
        this.narrationAudio = audio;
        if (title) title.textContent = typeof t === "function" ? t("🔊 正在语音播报") : "🔊 正在语音播报";
        document.getElementById("btn-narr-toggle").textContent = typeof t === "function" ? t("暂停") : "暂停";
        audio.onended = () => {
          URL.revokeObjectURL(url);
          this.stopNarration();
        };
        await audio.play();
      } catch (_) {
        // Browser speechSynthesis fallback
        if (typeof speechSynthesis !== "undefined") {
          const u = new SpeechSynthesisUtterance(cleaned);
          u.lang = (window.I18n && window.I18n.isZh && window.I18n.isZh())
            ? "zh-CN"
            : ((window.I18n && window.I18n.isJa && window.I18n.isJa()) ? "ja-JP" : "en-US");
          if (title) title.textContent = typeof t === "function" ? t("🔊 系统朗读中") : "🔊 系统朗读中";
          u.onend = () => this.stopNarration();
          speechSynthesis.speak(u);
        } else {
          showToast?.(typeof t === "function" ? t("当前浏览器不支持语音播报") : "当前浏览器不支持语音播报");
          this.stopNarration();
        }
      }
    },

    stopNarration() {
      if (this.narrationAudio) {
        try { this.narrationAudio.pause(); } catch (_) {}
        this.narrationAudio = null;
      }
      if (typeof speechSynthesis !== "undefined") {
        try { speechSynthesis.cancel(); } catch (_) {}
      }
      document.getElementById("narration-bar")?.classList.add("hidden");
    },

    // --- 8. <agent-embed> postMessage Bridge & sidecar:// links ---
    bindAgentEmbedBridge() {
      const parseEmbedNs = (rawUri) => {
        if (typeof window.parseMcpEmbedNamespace === "function") {
          return window.parseMcpEmbedNamespace(rawUri);
        }
        const s = String(rawUri || "").trim();
        const m = s.match(/^(ui|mcp):\/\/([^/?#]*)([^?#]*)(\?[^#]*)?/i);
        if (!m) return null;
        const host = (m[2] || "").trim();
        const q = m[4] ? new URLSearchParams(m[4].slice(1)) : null;
        const serverName = (q && (q.get("serverName") || q.get("server") || "").trim()) || host;
        return { scheme: m[1].toLowerCase(), serverName, appUri: s };
      };

      window.addEventListener("message", async (e) => {
        if (!e.source) return;
        if (e.origin && e.origin !== "null" && e.origin !== location.origin) return;

        // CRIT-03: Strictly verify e.source belongs to a mounted controlled iframe.agent-embed-frame
        let sourceCard = null;
        let sourceFrame = null;
        const frames = document.querySelectorAll("iframe.agent-embed-frame");
        for (const f of frames) {
          if (f.isConnected && f.contentWindow === e.source) {
            sourceFrame = f;
            sourceCard = f.closest(".agent-embed-card");
            break;
          }
        }
        if (!sourceCard || !sourceFrame) return;

        const d = e.data;
        if (!d || typeof d !== "object") return;
        const action = d.method || d.type;
        if (!action) return;
        const params = (d.params && typeof d.params === "object") ? d.params : d;
        const reply = (ok, result, error) => {
          if (!sourceFrame.isConnected) return;
          const msg = { id: d.id, ok };
          if (d.jsonrpc) msg.jsonrpc = d.jsonrpc;
          if (ok) msg.result = result;
          else msg.error = error;
          sourceFrame.contentWindow?.postMessage?.(msg, "*");
        };

        const cardSrc = sourceCard.getAttribute("data-src") || "";
        const cardCascadeId = sourceCard.getAttribute("data-cascade-id") || activeCascadeId || "";
        const embedNs = parseEmbedNs(cardSrc);
        const boundServer = sourceCard.getAttribute("data-mcp-server") || (embedNs ? embedNs.serverName : "");

        if ((action === "sendMessage" || action === "ui/message") && (params.text || params.message)) {
          // Ignore any iframe-supplied cascadeId; lock strictly to the embed's bound active conversation.
          if (!cardCascadeId || cardCascadeId !== activeCascadeId) {
            reply(false, null, "Cross-session message denied");
            return;
          }
          try {
            await rpc?.("SendUserCascadeMessage", {
              cascadeId: cardCascadeId,
              items: [{ text: String(params.text || params.message) }],
              model: activeModel || undefined,
            });
            reply(true, { ok: true });
          } catch (err) {
            reply(false, null, err.message);
          }
        } else if (action === "updateModelContext" && params.context) {
          if (!cardCascadeId || cardCascadeId !== activeCascadeId) {
            reply(false, null, "Cross-session context update denied");
            return;
          }
          this.contextItems.push({
            id: "embed-" + Date.now(),
            type: "file",
            label: String(params.label || "交互上下文").slice(0, 120),
            scopeItem: { codeContext: params.context },
          });
          this.renderContextChips();
          reply(true, { ok: true });
        } else if ((action === "callServerTool" || action === "tools/call") && (params.toolName || params.name)) {
          // Lock MCP tool invocation strictly to this embed's bound MCP App server namespace.
          if (!embedNs || !boundServer) {
            reply(false, null, "MCP tool bridge denied: embed is not bound to an MCP server");
            return;
          }
          const reqServer = params.serverName ? String(params.serverName).trim() : "";
          if (reqServer && reqServer !== boundServer) {
            reply(false, null, "Cross-server MCP tool call denied");
            return;
          }
          const reqUri = params.appUri || params.uri ? String(params.appUri || params.uri).trim() : "";
          if (reqUri) {
            const reqNs = parseEmbedNs(reqUri);
            if (!reqNs || (reqNs.serverName && reqNs.serverName !== boundServer)) {
              reply(false, null, "Cross-app MCP URI denied");
              return;
            }
          }
          try {
            const toolName = String(params.toolName || params.name);
            const res = await fetch("/gateway/mcp/app/call", {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({
                toolName,
                arguments: (params.arguments && typeof params.arguments === "object") ? params.arguments : {},
                appUri: cardSrc,
                serverName: boundServer,
              }),
            });
            const payload = await res.json().catch(() => ({}));
            reply(res.ok, payload, res.ok ? undefined : (payload.error || `HTTP ${res.status}`));
          } catch (err) {
            reply(false, null, err.message);
          }
        } else if (action === "resources/read" && params.uri) {
          if (!embedNs || !boundServer) {
            reply(false, null, "MCP resource bridge denied: embed is not bound to an MCP server");
            return;
          }
          const reqServer = params.serverName ? String(params.serverName).trim() : "";
          if (reqServer && reqServer !== boundServer) {
            reply(false, null, "Cross-server MCP resource read denied");
            return;
          }
          const reqUri = String(params.uri).trim();
          const reqNs = parseEmbedNs(reqUri);
          if (!reqNs || (reqNs.serverName && reqNs.serverName !== boundServer)) {
            reply(false, null, "Invalid or cross-server MCP resource URI");
            return;
          }
          try {
            const res = await fetch("/gateway/mcp/app/resource", {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ uri: reqUri, serverName: boundServer }),
            });
            const payload = await res.json().catch(() => ({}));
            reply(res.ok, payload, res.ok ? undefined : (payload.error || `HTTP ${res.status}`));
          } catch (err) {
            reply(false, null, err.message);
          }
        }
      });
    },

    bindSidecarLinks() {
      document.addEventListener("click", (e) => {
        const link = e.target.closest('a[href^="sidecar://"]');
        if (!link) return;
        e.preventDefault();
        const raw = link.getAttribute("href") || "";
        const rest = raw.replace(/^sidecar:\/\//i, "").replace(/^\/+/, "");
        const slashIdx = rest.indexOf("/");
        const sid = slashIdx >= 0 ? rest.slice(0, slashIdx) : rest;
        const sub = slashIdx >= 0 ? rest.slice(slashIdx) : "/";
        if (!sid || sid === "." || sid === ".." || !/^[A-Za-z0-9_.-]+$/.test(sid)) return;
        if (sid.toLowerCase() === "swarm" || sub.toLowerCase().includes("swarm_board")) {
          if (typeof SwarmManager !== "undefined" && SwarmManager.toggleSwarmModal) {
            SwarmManager.toggleSwarmModal(true);
            return;
          }
        }
        if (/^(dashboard|automations|sidecars|schedule|schedules)$/i.test(sid)) {
          this._selectedAutomationId = null;
          this.openToolsBoard("automations");
          return;
        }
        // Normalize subpath and block any '..' path traversal escaping /gateway/sidecar-ui/<sid>/
        let decodedSub = sub;
        try { decodedSub = decodeURIComponent(sub); } catch (_) {}
        if (/(?:^|[\\/])\.\.(?:[\\/]|$|[?#])/.test(sub) || /(?:^|[\\/])\.\.(?:[\\/]|$|[?#])/.test(decodedSub)) {
          return;
        }
        let normSub;
        try {
          normSub = new URL(sub.startsWith("/") ? sub : "/" + sub, "https://sidecar.invalid");
        } catch (_) {
          return;
        }
        const basePrefix = `/gateway/sidecar-ui/${encodeURIComponent(sid)}/`;
        const targetUrl = `/gateway/sidecar-ui/${encodeURIComponent(sid)}${normSub.pathname}${normSub.search}${normSub.hash}`;
        try {
          const checkUrl = new URL(targetUrl, location.origin);
          if (!checkUrl.pathname.startsWith(basePrefix)) return;
        } catch (_) {
          return;
        }
        this.openSidecarSheet(sid, targetUrl);
      });
    },

    startAutomationWithPrompt() {
      this._selectedAutomationId = null;
      this._creatingAutomation = false;
      this._openAutomationPromptEditor = false;
      this.closeToolsBoard();
      const promptText = typeof t === "function" ? t("引导我创建一个定时自动化任务") : "引导我创建一个定时自动化任务";
      if (typeof ensureSlashCommands === "function") ensureSlashCommands();
      if (typeof startDraftSession === "function") {
        startDraftSession({
          isPure: true,
          name: "新对话",
          path: "",
          uri: "",
          rawId: "outside-of-project",
          prefillSlashCommands: [{ name: "automation", title: "automation", kind: "skill", icon: "schedule" }],
          prefillText: promptText,
        });
      }
    },

    _autoSlugify(name) {
      return String(name || "")
        .toLowerCase()
        .trim()
        .replace(/[^a-z0-9]+/g, "-")
        .replace(/^-+|-+$/g, "");
    },

    _parseAutoNextRunMs(str) {
      if (!str) return Number.MAX_SAFE_INTEGER;
      const ms = Date.parse(String(str).replace(" ", "T"));
      return Number.isFinite(ms) ? ms : Number.MAX_SAFE_INTEGER;
    },

    _extractAutoDataSources(prompt) {
      const text = String(prompt || "");
      if (!text) return [];
      const found = [];
      const pushUnique = (label) => {
        const clean = String(label || "").trim();
        if (!clean || found.includes(clean)) return;
        found.push(clean);
      };
      const known = [
        [/\bgithub\b|github\.com/i, "GitHub"],
        [/\bslack\b/i, "Slack"],
        [/\bgmail\b|google mail/i, "Gmail"],
        [/google calendar|\bcalendar\b/i, "Calendar"],
        [/\blinear\b/i, "Linear"],
        [/\bnotion\b/i, "Notion"],
        [/\bjira\b/i, "Jira"],
        [/\bdiscord\b/i, "Discord"],
        [/\btelegram\b/i, "Telegram"],
        [/\bwechat\b|微信/i, "WeChat"],
        [/\bsubstack\b/i, "Substack"],
        [/hacker\s*news|news\.ycombinator/i, "Hacker News"],
        [/\breddit\b/i, "Reddit"],
        [/\btwi(?:tter)?\b|x\.com/i, "X / Twitter"],
      ];
      for (const [re, label] of known) {
        if (re.test(text)) pushUnique(label);
      }
      const urls = text.match(/https?:\/\/([a-z0-9.-]+\.[a-z]{2,})/gi) || [];
      for (const u of urls.slice(0, 4)) {
        try {
          const host = new URL(u).hostname.replace(/^www\./, "");
          if (host) pushUnique(host);
        } catch (_) {}
      }
      return found.slice(0, 5);
    },

    _defaultAutoTimeZone() {
      try {
        return Intl.DateTimeFormat().resolvedOptions().timeZone || "";
      } catch (_) {
        return "";
      }
    },

    _parseAutoCron(rawExpr, fallbackTz) {
      let expr = String(rawExpr || "").trim();
      let timeZone = String(fallbackTz || "").trim();
      const tzMatch = expr.match(/^(?:CRON_TZ|TZ)=([^\s]+)\s+(.+)$/);
      if (tzMatch) {
        timeZone = tzMatch[1];
        expr = tzMatch[2].trim();
      }
      const base = {
        preset: "daily", // hourly | daily | weekdays | weekly | custom
        customKind: "every_hours", // every_hours | multi_days | monthly | raw
        hour: 9,
        minute: 0,
        weekday: 1,
        weekdays: [1, 3, 5],
        everyHours: 2,
        dayOfMonth: 1,
        timeZone: timeZone || this._defaultAutoTimeZone(),
        rawCron: expr || "0 9 * * *",
      };
      const parts = expr.split(/\s+/);
      if (parts.length !== 5) {
        return { ...base, preset: "custom", customKind: "raw", rawCron: expr || "0 9 * * *" };
      }
      const [minS, hourS, domS, monS, dowS] = parts;
      const minNum = /^\d+$/.test(minS) ? Number(minS) : NaN;
      const hourNum = /^\d+$/.test(hourS) ? Number(hourS) : NaN;

      if (minS === "0" && hourS === "*" && domS === "*" && monS === "*" && dowS === "*") {
        return { ...base, preset: "hourly", minute: 0 };
      }
      const everyHMatch = hourS.match(/^\*\/(\d+)$/);
      if (minS === "0" && everyHMatch && domS === "*" && monS === "*" && dowS === "*") {
        const n = Number(everyHMatch[1]);
        if ([2, 3, 4, 6, 8, 12].includes(n)) {
          return { ...base, preset: "custom", customKind: "every_hours", everyHours: n };
        }
      }
      if (Number.isFinite(minNum) && Number.isFinite(hourNum) && monS === "*") {
        if (domS === "*" && dowS === "*") {
          return { ...base, preset: "daily", hour: hourNum, minute: minNum };
        }
        if (domS === "*" && dowS === "1-5") {
          return { ...base, preset: "weekdays", hour: hourNum, minute: minNum };
        }
        if (domS === "*" && /^[0-7]$/.test(dowS)) {
          return { ...base, preset: "weekly", hour: hourNum, minute: minNum, weekday: Number(dowS) % 7 };
        }
        if (domS === "*" && /^[0-7](,[0-7])+$/.test(dowS)) {
          const days = Array.from(new Set(dowS.split(",").map((d) => Number(d) % 7))).sort((a, b) => a - b);
          return { ...base, preset: "custom", customKind: "multi_days", hour: hourNum, minute: minNum, weekdays: days };
        }
        if (/^\d+$/.test(domS) && dowS === "*") {
          const dom = Math.min(28, Math.max(1, Number(domS)));
          return { ...base, preset: "custom", customKind: "monthly", hour: hourNum, minute: minNum, dayOfMonth: dom };
        }
      }
      return { ...base, preset: "custom", customKind: "raw", rawCron: expr };
    },

    _buildAutoCron(draft) {
      const m = Math.min(59, Math.max(0, Number(draft.minute) || 0));
      const h = Math.min(23, Math.max(0, Number(draft.hour) || 0));
      let core = "";
      if (draft.preset === "hourly") {
        core = "0 * * * *";
      } else if (draft.preset === "daily") {
        core = `${m} ${h} * * *`;
      } else if (draft.preset === "weekdays") {
        core = `${m} ${h} * * 1-5`;
      } else if (draft.preset === "weekly") {
        core = `${m} ${h} * * ${Number(draft.weekday) % 7}`;
      } else if (draft.customKind === "every_hours") {
        core = `0 */${draft.everyHours || 2} * * *`;
      } else if (draft.customKind === "multi_days") {
        const days = (draft.weekdays && draft.weekdays.length ? draft.weekdays : [1]).slice().sort((a, b) => a - b);
        core = `${m} ${h} * * ${days.join(",")}`;
      } else if (draft.customKind === "monthly") {
        const dom = Math.min(28, Math.max(1, Number(draft.dayOfMonth) || 1));
        core = `${m} ${h} ${dom} * *`;
      } else {
        core = String(draft.rawCron || "").trim();
        if (/^(?:CRON_TZ|TZ)=/.test(core)) return core;
      }
      if (!core) return "";
      const tz = String(draft.timeZone || "").trim();
      return tz ? `CRON_TZ=${tz} ${core}` : core;
    },

    _humanizeAutoCron(rawExpr) {
      const expr = String(rawExpr || "").trim();
      if (!expr) return "";
      const d = this._parseAutoCron(expr, "");
      const pad = (n) => String(n).padStart(2, "0");
      const timeStr = `${pad(d.hour)}:${pad(d.minute)}`;
      const isChinese = window.I18n?.isZh?.();
      const isJapanese = window.I18n?.isJa?.();
      const dayNames = isChinese
        ? ["周日", "周一", "周二", "周三", "周四", "周五", "周六"]
        : (isJapanese
          ? ["日曜", "月曜", "火曜", "水曜", "木曜", "金曜", "土曜"]
          : ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"]);
      let label = expr;
      if (d.preset === "hourly") {
        label = isChinese ? "每小时" : (isJapanese ? "毎時" : "Hourly");
      } else if (d.preset === "daily") {
        label = isChinese ? `每天 ${timeStr}` : (isJapanese ? `毎日 ${timeStr}` : `Daily ${timeStr}`);
      } else if (d.preset === "weekdays") {
        label = isChinese ? `工作日 ${timeStr}` : (isJapanese ? `平日 ${timeStr}` : `Weekdays ${timeStr}`);
      } else if (d.preset === "weekly") {
        label = isChinese
          ? `每${dayNames[d.weekday % 7]} ${timeStr}`
          : (isJapanese ? `毎週${dayNames[d.weekday % 7]} ${timeStr}` : `Every ${dayNames[d.weekday % 7]} ${timeStr}`);
      } else if (d.customKind === "every_hours") {
        label = isChinese ? `每 ${d.everyHours} 小时` : (isJapanese ? `${d.everyHours} 時間ごと` : `Every ${d.everyHours}h`);
      } else if (d.customKind === "multi_days") {
        const sep = isChinese ? "、" : (isJapanese ? "・" : ", ");
        const days = d.weekdays.map((x) => dayNames[x % 7]).join(sep);
        label = isChinese ? `每${days} ${timeStr}` : (isJapanese ? `毎週${days} ${timeStr}` : `${days} ${timeStr}`);
      } else if (d.customKind === "monthly") {
        label = isChinese ? `每月 ${d.dayOfMonth} 日 ${timeStr}` : (isJapanese ? `毎月 ${d.dayOfMonth} 日 ${timeStr}` : `Monthly day ${d.dayOfMonth} ${timeStr}`);
      } else {
        label = d.rawCron || expr;
      }
      const hasExplicitTz = /^(?:CRON_TZ|TZ)=([^\s]+)\s+/.exec(expr);
      return hasExplicitTz ? `${label} (${hasExplicitTz[1]})` : label;
    },

    _renderAutoScheduleEditorHtml(prefix, draft) {
      const pad = (n) => String(n).padStart(2, "0");
      const timeVal = `${pad(draft.hour)}:${pad(draft.minute)}`;
      const presets = [
        ["hourly", "每小时"],
        ["daily", "每天"],
        ["weekdays", "工作日"],
        ["weekly", "每周"],
        ["custom", "自定义"],
      ];
      const isChinese = window.I18n?.isZh?.();
      const isJapanese = window.I18n?.isJa?.();
      const dayLabels = isChinese
        ? ["日", "一", "二", "三", "四", "五", "六"]
        : (isJapanese
          ? ["日", "月", "火", "水", "木", "金", "土"]
          : ["Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"]);
      const rawCronPh = typeof t === "function" ? t("如 0 9 * * 1-5") : "如 0 9 * * 1-5";
      const tzPh = typeof t === "function" ? t("如 Asia/Shanghai") : "如 Asia/Shanghai";
      const needTime = draft.preset === "daily" || draft.preset === "weekdays" || draft.preset === "weekly" || (draft.preset === "custom" && (draft.customKind === "multi_days" || draft.customKind === "monthly"));
      const builtCron = this._buildAutoCron(draft);
      return `
        <div class="p3-auto-sched" data-sched-prefix="${prefix}">
          <div class="p3-auto-pill-row">
            ${presets.map(([k, lbl]) => `<button type="button" class="p3-auto-pill ${draft.preset === k ? "active" : ""}" data-sched-preset="${k}">${lbl}</button>`).join("")}
          </div>
          ${draft.preset === "weekly" ? `
            <div class="p3-auto-pill-row" style="margin-top:6px;">
              ${dayLabels.map((lbl, idx) => `<button type="button" class="p3-auto-day-chip ${draft.weekday === idx ? "active" : ""}" data-sched-weekday="${idx}">${lbl}</button>`).join("")}
            </div>
          ` : ""}
          ${draft.preset === "custom" ? `
            <div class="p3-auto-pill-row" style="margin-top:6px;">
              <button type="button" class="p3-auto-pill ${draft.customKind === "every_hours" ? "active" : ""}" data-sched-custom="every_hours">每隔数小时</button>
              <button type="button" class="p3-auto-pill ${draft.customKind === "multi_days" ? "active" : ""}" data-sched-custom="multi_days">每周多日</button>
              <button type="button" class="p3-auto-pill ${draft.customKind === "monthly" ? "active" : ""}" data-sched-custom="monthly">每月</button>
              <button type="button" class="p3-auto-pill ${draft.customKind === "raw" ? "active" : ""}" data-sched-custom="raw">原始 Cron</button>
            </div>
            ${draft.customKind === "every_hours" ? `
              <div class="p3-auto-pill-row" style="margin-top:6px;">
                ${[2, 3, 4, 6, 8, 12].map((n) => `<button type="button" class="p3-auto-pill ${draft.everyHours === n ? "active" : ""}" data-sched-every-h="${n}">每 ${n} 小时</button>`).join("")}
              </div>
            ` : ""}
            ${draft.customKind === "multi_days" ? `
              <div class="p3-auto-pill-row" style="margin-top:6px;">
                ${dayLabels.map((lbl, idx) => `<button type="button" class="p3-auto-day-chip ${(draft.weekdays || []).includes(idx) ? "active" : ""}" data-sched-multiday="${idx}">${lbl}</button>`).join("")}
              </div>
            ` : ""}
            ${draft.customKind === "monthly" ? `
              <div class="p3-auto-inline-row" style="margin-top:6px;">
                <span class="p3-auto-field-label">每月几日 (1-28)</span>
                <input type="number" min="1" max="28" class="p3-input p3-auto-compact-input" data-sched-dom value="${draft.dayOfMonth || 1}" />
              </div>
            ` : ""}
            ${draft.customKind === "raw" ? `
              <input type="text" class="p3-input monospaced" style="margin-top:6px;" data-sched-raw value="${escapeHtml(draft.rawCron || "")}" placeholder="${escapeHtml(rawCronPh)}" />
            ` : ""}
          ` : ""}
          <div class="p3-auto-inline-grid" style="margin-top:8px;">
            ${needTime ? `
              <div>
                <span class="p3-auto-field-label">执行时间</span>
                <input type="time" class="p3-input" data-sched-time value="${timeVal}" />
              </div>
            ` : ""}
            <div style="flex:1;">
              <span class="p3-auto-field-label">时区（IANA，可选）</span>
              <input type="text" class="p3-input monospaced" data-sched-tz value="${escapeHtml(draft.timeZone || "")}" placeholder="${escapeHtml(tzPh)}" />
            </div>
          </div>
          <div class="p3-auto-cron-preview monospaced" data-sched-preview>
            <span>${escapeHtml(this._humanizeAutoCron(builtCron) || builtCron)}</span>
            <span style="opacity:.65;">${escapeHtml(builtCron)}</span>
          </div>
        </div>`;
    },

    _mountAutoScheduleEditor(container, prefix, draft, onChange) {
      if (!container) return;
      const render = () => {
        container.innerHTML = this._renderAutoScheduleEditorHtml(prefix, draft);
        const notify = () => {
          const built = this._buildAutoCron(draft);
          const prev = container.querySelector("[data-sched-preview]");
          if (prev) {
            prev.innerHTML = `<span>${escapeHtml(this._humanizeAutoCron(built) || built)}</span><span style="opacity:.65;">${escapeHtml(built)}</span>`;
          }
          onChange?.(built);
        };
        container.querySelectorAll("[data-sched-preset]").forEach((btn) => {
          btn.addEventListener("click", () => {
            draft.preset = btn.getAttribute("data-sched-preset");
            render();
            onChange?.(this._buildAutoCron(draft));
          });
        });
        container.querySelectorAll("[data-sched-custom]").forEach((btn) => {
          btn.addEventListener("click", () => {
            draft.customKind = btn.getAttribute("data-sched-custom");
            render();
            onChange?.(this._buildAutoCron(draft));
          });
        });
        container.querySelectorAll("[data-sched-weekday]").forEach((btn) => {
          btn.addEventListener("click", () => {
            draft.weekday = Number(btn.getAttribute("data-sched-weekday"));
            render();
            onChange?.(this._buildAutoCron(draft));
          });
        });
        container.querySelectorAll("[data-sched-multiday]").forEach((btn) => {
          btn.addEventListener("click", () => {
            const d = Number(btn.getAttribute("data-sched-multiday"));
            const set = new Set(draft.weekdays || []);
            if (set.has(d)) {
              if (set.size > 1) set.delete(d);
            } else {
              set.add(d);
            }
            draft.weekdays = Array.from(set).sort((a, b) => a - b);
            render();
            onChange?.(this._buildAutoCron(draft));
          });
        });
        container.querySelectorAll("[data-sched-every-h]").forEach((btn) => {
          btn.addEventListener("click", () => {
            draft.everyHours = Number(btn.getAttribute("data-sched-every-h"));
            render();
            onChange?.(this._buildAutoCron(draft));
          });
        });
        container.querySelector("[data-sched-dom]")?.addEventListener("input", (e) => {
          draft.dayOfMonth = Math.min(28, Math.max(1, Number(e.target.value) || 1));
          notify();
        });
        container.querySelector("[data-sched-raw]")?.addEventListener("input", (e) => {
          draft.rawCron = e.target.value;
          notify();
        });
        container.querySelector("[data-sched-time]")?.addEventListener("input", (e) => {
          const [hh, mm] = String(e.target.value || "09:00").split(":").map(Number);
          if (Number.isFinite(hh)) draft.hour = hh;
          if (Number.isFinite(mm)) draft.minute = mm;
          notify();
        });
        container.querySelector("[data-sched-tz]")?.addEventListener("input", (e) => {
          draft.timeZone = e.target.value.trim();
          notify();
        });
      };
      render();
    },

    async _fetchAutoPickerDeps() {
      const [mRes, pRes] = await Promise.all([
        fetch("/gateway/models").catch(() => null),
        fetch("/gateway/projects").catch(() => null),
      ]);
      const mData = mRes ? await mRes.json().catch(() => ({})) : {};
      const pData = pRes ? await pRes.json().catch(() => ({})) : {};
      const models = Array.isArray(mData.models) ? mData.models : [];
      const rawProjects = Array.isArray(pData.projects) ? pData.projects : (Array.isArray(pData) ? pData : []);
      const seen = new Set();
      const projects = [];
      for (const p of rawProjects) {
        const pid = String(p.projectId || p.id || "").trim();
        if (!pid || pid === "outside-of-project" || seen.has(pid)) continue;
        seen.add(pid);
        const label = p.name || (p.path ? p.path.split("/").filter(Boolean).pop() : "") || pid;
        projects.push({ id: pid, name: label });
      }
      return { models, projects };
    },

    async renderCreateAutomationForm(body, existingList) {
      const { models, projects } = await this._fetchAutoPickerDeps();
      const existingIds = new Set((existingList || []).map((x) => String(x.id || "").toLowerCase()));
      const draft = this._parseAutoCron("0 9 * * *", this._defaultAutoTimeZone());

      const modelOptionsHtml = [
        `<option value="">默认模型</option>`,
        ...models.map((m) => `<option value="${escapeHtml(m.id)}">${escapeHtml(m.name || m.id)}</option>`),
      ].join("");
      const projectOptionsHtml = [
        `<option value="">不绑定项目（独立运行）</option>`,
        ...projects.map((p) => `<option value="${escapeHtml(p.id)}">${escapeHtml(p.name)}</option>`),
      ].join("");

      body.innerHTML = `
        <div class="p3-section-header" style="margin-bottom:10px;">
          <button type="button" class="p3-mini-btn" id="btn-auto-create-back">← 返回列表</button>
          <span style="font-weight:600;">手动新建定时任务</span>
        </div>
        <div class="p3-form-card">
          <label class="p3-auto-field-label">任务名称</label>
          <input type="text" id="p3-create-auto-name" class="p3-input" placeholder="任务名称 (如 晨间代码巡检)" />
          <div id="p3-create-auto-slug-hint" class="p3-auto-slug-hint hidden"></div>

          <label class="p3-auto-field-label" style="margin-top:10px;">触发计划</label>
          <div id="p3-create-auto-sched-mount"></div>

          <div class="p3-auto-inline-grid" style="margin-top:10px;">
            <div style="flex:1;">
              <label class="p3-auto-field-label">执行模型</label>
              <select id="p3-create-auto-model" class="p3-input">${modelOptionsHtml}</select>
            </div>
            <div style="flex:1;">
              <label class="p3-auto-field-label">所属项目</label>
              <select id="p3-create-auto-project" class="p3-input">${projectOptionsHtml}</select>
            </div>
          </div>

          <label class="p3-auto-field-label" style="margin-top:10px;">执行指令（Prompt）</label>
          <textarea id="p3-create-auto-prompt" class="p3-textarea" rows="6" placeholder="描述任务触发时 Agent 需要完成的具体工作与输出格式…"></textarea>

          <div class="p3-row" style="margin-top:10px;">
            <span style="font-size:12.5px;opacity:.85;">创建后立即启用</span>
            <label class="wf-switch"><input type="checkbox" id="p3-create-auto-enabled" checked><span class="wf-switch-slider"></span></label>
          </div>

          <div class="p3-form-actions" style="margin-top:12px;">
            <button type="button" class="p3-mini-btn" id="btn-auto-create-cancel">取消</button>
            <button type="button" class="p3-mini-btn primary" id="btn-auto-create-submit">创建任务</button>
          </div>
        </div>`;

      this._mountAutoScheduleEditor(document.getElementById("p3-create-auto-sched-mount"), "create", draft);

      const nameInput = document.getElementById("p3-create-auto-name");
      const slugHint = document.getElementById("p3-create-auto-slug-hint");
      const updateSlugHint = () => {
        const slug = this._autoSlugify(nameInput?.value || "");
        if (!slugHint) return false;
        if (!slug) {
          slugHint.classList.add("hidden");
          slugHint.textContent = "";
          return false;
        }
        const dup = existingIds.has(slug);
        slugHint.classList.remove("hidden");
        slugHint.classList.toggle("error", dup);
        slugHint.textContent = dup ? `ID「${slug}」已存在，请换一个任务名称` : `ID: ${slug}`;
        return dup;
      };
      nameInput?.addEventListener("input", updateSlugHint);

      const leaveCreate = () => {
        this._creatingAutomation = false;
        this.renderToolsTab(false);
      };
      document.getElementById("btn-auto-create-back")?.addEventListener("click", leaveCreate);
      document.getElementById("btn-auto-create-cancel")?.addEventListener("click", leaveCreate);
      document.getElementById("btn-auto-create-submit")?.addEventListener("click", async () => {
        const name = nameInput?.value.trim() || "";
        const cron = this._buildAutoCron(draft).trim();
        const model = document.getElementById("p3-create-auto-model")?.value || "";
        const projectId = document.getElementById("p3-create-auto-project")?.value || "";
        const prompt = document.getElementById("p3-create-auto-prompt")?.value.trim() || "";
        const enabled = !!document.getElementById("p3-create-auto-enabled")?.checked;
        if (!name || !cron || !prompt) {
          showToast?.("请填写任务名称、触发计划和执行指令");
          return;
        }
        if (updateSlugHint()) {
          showToast?.("已存在同 ID 的定时任务");
          return;
        }
        const btn = document.getElementById("btn-auto-create-submit");
        if (btn) btn.disabled = true;
        const r = await fetch("/gateway/automations/create", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ name, cron, model, projectId, prompt, enabled }),
        });
        const d = await r.json().catch(() => ({}));
        if (!r.ok || d.ok === false) {
          if (btn) btn.disabled = false;
          showToast?.("创建失败: " + (d.error || r.status));
          return;
        }
        this._creatingAutomation = false;
        showToast?.("已创建定时任务");
        this.renderToolsTab(false);
      });
    },

    async renderAutomationDetail(body, sc, allEvents, openPromptImmediately = false) {
      const running = !!(sc.enabled || /^(running|ready)$/i.test(sc.status || ""));
      const taskEvents = (allEvents || []).filter((ev) =>
        (ev.sidecarId && ev.sidecarId === sc.id) || ev.scheduleId === sc.id || ev.scheduleId === sc.name
      );
      const { models, projects } = await this._fetchAutoPickerDeps();

      const modelLabel = (models.find((m) => m.id === sc.model)?.name) || sc.model || "默认模型";
      const wsName = sc.projectName || (sc.workspaceUri ? sc.workspaceUri.replace(/^file:\/\//, "").split("/").filter(Boolean).pop() : "") || "不绑定项目";
      const scheduleLabel = this._humanizeAutoCron(sc.cronExpression || "") || sc.cronExpression || "未设置";
      const dataSources = this._extractAutoDataSources(sc.prompt || "");
      const promptRaw = String(sc.prompt || "");
      const isLongPrompt = promptRaw.length > 480;
      const promptRenderedHtml = promptRaw
        ? (typeof renderMarkdown === "function" ? renderMarkdown(promptRaw) : `<div style="white-space:pre-wrap;">${escapeHtml(promptRaw)}</div>`)
        : `<div style="opacity:.6;">${escapeHtml(typeof t === "function" ? t("暂无执行指令") : "暂无执行指令")}</div>`;

      const modelOptionsHtml = [
        `<option value="">默认模型</option>`,
        ...models.map((m) => `<option value="${escapeHtml(m.id)}" ${sc.model === m.id ? "selected" : ""}>${escapeHtml(m.name || m.id)}</option>`),
        ...(sc.model && !models.some((m) => m.id === sc.model) ? [`<option value="${escapeHtml(sc.model)}" selected>${escapeHtml(sc.model)}</option>`] : []),
      ].join("");

      const projectOptionsHtml = [
        `<option value="">不绑定项目（独立运行）</option>`,
        ...projects.map((p) => `<option value="${escapeHtml(p.id)}" ${sc.projectId === p.id ? "selected" : ""}>${escapeHtml(p.name)}</option>`),
        ...(sc.projectId && !projects.some((p) => p.id === sc.projectId) ? [`<option value="${escapeHtml(sc.projectId)}" selected>${escapeHtml(sc.projectName || sc.projectId)}</option>`] : []),
      ].join("");

      const statusLabelMap = {
        running: "运行中",
        needs_input: "等待确认",
        failed: "失败",
        ok: "已完成",
      };

      body.innerHTML = `
        <div class="p3-section-header" style="margin-bottom:10px;">
          <button type="button" class="p3-mini-btn" id="btn-auto-back">← 返回列表</button>
          <div style="display:flex;gap:6px;align-items:center;">
            <button type="button" class="p3-mini-btn" id="btn-auto-detail-logs">日志</button>
            ${running ? `<button type="button" class="p3-mini-btn" id="btn-auto-detail-restart">重启</button>` : ""}
            <button type="button" class="p3-mini-btn danger" id="btn-auto-detail-del">删除</button>
          </div>
        </div>

        <div class="p3-form-card" style="margin-bottom:10px;">
          <div style="display:flex;align-items:flex-start;justify-content:space-between;gap:10px;">
            <div style="flex:1;min-width:0;">
              <div id="p3-auto-title-view" style="display:flex;align-items:center;gap:6px;">
                <span style="font-size:16px;font-weight:700;word-break:break-word;">${escapeHtml(sc.name || sc.id)}</span>
                <button type="button" class="p3-mini-btn p3-auto-icon-btn" id="btn-auto-title-edit" title="修改名称">✎</button>
              </div>
              <div id="p3-auto-title-editor" class="hidden" style="display:flex;gap:6px;align-items:center;">
                <input type="text" id="p3-edit-auto-name" class="p3-input" value="${escapeHtml(sc.name || "")}" style="margin:0;flex:1;" />
                <button type="button" class="p3-mini-btn primary" id="btn-auto-title-save">保存</button>
                <button type="button" class="p3-mini-btn" id="btn-auto-title-cancel">取消</button>
              </div>
              <div style="font-size:11.5px;opacity:.75;margin-top:4px;">
                <span class="p3-badge ${running ? "ok" : ""}">${running ? "计划运行" : "已暂停"}</span>
                ${running && sc.nextRunAt ? ` · ${escapeHtml(typeof t === "function" ? t("下次") : "下次")} ${escapeHtml(sc.nextRunAt)}` : ""}
                ${sc.lastRunAt ? ` · ${escapeHtml(typeof t === "function" ? t("上次") : "上次")} ${escapeHtml(sc.lastRunAt)}` : ""}
              </div>
            </div>
            <div style="display:flex;gap:8px;align-items:center;flex-shrink:0;">
              <button type="button" class="p3-mini-btn" id="btn-auto-detail-run" ${running ? "" : "disabled"} title="${running ? "立即运行" : "请先启动任务"}">▶ 运行</button>
              <label class="wf-switch" title="${running ? "暂停" : "启用"}">
                <input type="checkbox" id="p3-auto-detail-switch" ${running ? "checked" : ""}>
                <span class="wf-switch-slider"></span>
              </label>
            </div>
          </div>
          ${sc.lastError ? `<div style="color:#ef4444;font-size:12px;padding:6px 8px;border-radius:8px;background:rgba(239,68,68,.1);margin-top:8px;">${escapeHtml(sc.lastError)}</div>` : ""}
        </div>

        <div class="p3-form-card" style="margin-bottom:10px;">
          <div class="p3-auto-config-summary" id="btn-auto-config-toggle" role="button" tabindex="0">
            <div style="flex:1;min-width:0;">
              <div style="font-size:12px;font-weight:600;opacity:.7;margin-bottom:4px;">运行配置（点击编辑）</div>
              <div class="p3-auto-config-chips">
                <span class="p3-auto-chip">⏱ ${escapeHtml(scheduleLabel)}</span>
                <span class="p3-auto-chip">🤖 ${escapeHtml(modelLabel)}</span>
                <span class="p3-auto-chip">📁 ${escapeHtml(wsName)}</span>
              </div>
            </div>
            <span id="p3-auto-config-chevron" style="opacity:.55;font-size:13px;">✎</span>
          </div>
          <div id="p3-auto-config-editor" class="hidden" style="margin-top:10px;padding-top:10px;border-top:1px solid rgba(120,120,128,.2);">
            <label class="p3-auto-field-label">触发计划</label>
            <div id="p3-edit-auto-sched-mount"></div>
            <div class="p3-auto-inline-grid" style="margin-top:10px;">
              <div style="flex:1;">
                <label class="p3-auto-field-label">执行模型</label>
                <select id="p3-edit-auto-model" class="p3-input">${modelOptionsHtml}</select>
              </div>
              <div style="flex:1;">
                <label class="p3-auto-field-label">所属项目</label>
                <select id="p3-edit-auto-project" class="p3-input">${projectOptionsHtml}</select>
              </div>
            </div>
            <div class="p3-form-actions" style="margin-top:10px;">
              <button type="button" class="p3-mini-btn" id="btn-auto-config-cancel">取消</button>
              <button type="button" class="p3-mini-btn primary" id="btn-auto-config-save">保存配置</button>
            </div>
          </div>
        </div>

        <div class="p3-form-card" style="margin-bottom:10px;">
          <div style="display:flex;align-items:center;justify-content:space-between;gap:8px;margin-bottom:8px;">
            <span style="font-size:13px;font-weight:600;">执行指令（Prompt）</span>
            <button type="button" class="p3-mini-btn primary" id="btn-auto-prompt-edit">✎ 编辑指令</button>
          </div>
          <div id="p3-auto-prompt-view">
            <div id="p3-auto-prompt-md" class="p3-auto-prompt-md ${isLongPrompt ? "collapsed" : ""}" data-i18n-skip="1">${promptRenderedHtml}</div>
            ${isLongPrompt ? `<button type="button" class="p3-mini-btn" id="btn-auto-prompt-expand" style="margin-top:6px;">展开完整指令</button>` : ""}
            ${dataSources.length > 0 ? `
              <div class="p3-auto-sources">
                <span style="opacity:.65;">读取：</span>
                ${dataSources.map((src) => `<span class="p3-auto-chip">${escapeHtml(src)}</span>`).join("")}
              </div>
            ` : ""}
          </div>
          <div id="p3-auto-prompt-editor" class="hidden">
            <textarea id="p3-edit-auto-prompt" class="p3-textarea" rows="10" placeholder="输入定时任务触发时交给 Agent 的完整指令…">${escapeHtml(promptRaw)}</textarea>
            <div class="p3-form-actions" style="margin-top:8px;">
              <button type="button" class="p3-mini-btn" id="btn-auto-prompt-cancel">取消</button>
              <button type="button" class="p3-mini-btn primary" id="btn-auto-prompt-save">保存指令</button>
            </div>
          </div>
        </div>

        <div id="p3-auto-logs-box" class="p3-form-card hidden" style="margin-bottom:10px;">
          <div style="display:flex;justify-content:space-between;align-items:center;gap:6px;margin-bottom:6px;flex-wrap:wrap;">
            <span style="font-size:12px;font-weight:600;">运行日志</span>
            <div style="display:flex;gap:6px;align-items:center;">
              <select id="p3-auto-logs-file" class="p3-input hidden" style="width:auto;margin:0;padding:3px 6px;font-size:11.5px;"></select>
              <button type="button" class="p3-mini-btn" id="btn-auto-logs-refresh">刷新</button>
            </div>
          </div>
          <pre id="p3-auto-logs-pre" style="margin:0;max-height:240px;overflow:auto;font-size:11px;line-height:1.45;white-space:pre-wrap;word-break:break-all;opacity:.9;"></pre>
        </div>

        <div class="p3-section-header" style="margin-top:6px;">
          <span>运行历史 (${taskEvents.length})</span>
        </div>
        <div class="p3-list">` + (taskEvents.slice(0, 15).map((ev) => {
          const st = ev.runStatus || (ev.status === "error" ? "failed" : "ok");
          const stLabel = statusLabelMap[st] || statusLabelMap.ok;
          const title = ev.conversationTitle || ev.timestamp || "自动化执行";
          return `
          <div class="p3-row" ${ev.conversationId ? `data-auto-conv="${escapeHtml(ev.conversationId)}" data-auto-conv-exists="${ev.conversationExists === false ? "0" : "1"}" style="cursor:pointer;"` : ""}>
            <div class="p3-row-main">
              <div class="p3-row-title" style="font-size:13px;display:flex;align-items:center;gap:6px;">
                <span class="p3-auto-status-dot ${escapeHtml(st)}"></span>
                <span>${escapeHtml(title)}</span>
                <span class="p3-badge p3-auto-st-badge ${escapeHtml(st)}">${escapeHtml(stLabel)}</span>
              </div>
              <div class="p3-row-sub monospaced" style="font-size:11px;opacity:.7;">${escapeHtml(ev.timestamp || "")}</div>
              ${ev.message ? `<div class="p3-row-sub">${escapeHtml(ev.message.slice(0, 120))}</div>` : ""}
            </div>
            ${ev.conversationId ? `<span style="opacity:.5;font-size:13px;">›</span>` : ""}
          </div>`;
        }).join("") || `<div class="p3-empty">${running ? "暂无运行记录" : "任务已暂停"}</div>`) + `</div>`;

      const schedDraft = this._parseAutoCron(sc.cronExpression || "", sc.timeZone || "");
      let schedMounted = false;
      let promptEditing = false;

      const isPromptDirty = () => {
        if (!promptEditing) return false;
        const cur = document.getElementById("p3-edit-auto-prompt")?.value ?? promptRaw;
        return cur !== promptRaw;
      };

      const setPromptEditing = (editing) => {
        promptEditing = editing;
        document.getElementById("p3-auto-prompt-view")?.classList.toggle("hidden", editing);
        document.getElementById("p3-auto-prompt-editor")?.classList.toggle("hidden", !editing);
        document.getElementById("btn-auto-prompt-edit")?.classList.toggle("hidden", editing);
        if (editing) {
          const ta = document.getElementById("p3-edit-auto-prompt");
          if (ta) {
            ta.value = promptRaw;
            setTimeout(() => ta.focus(), 60);
          }
        }
      };

      if (openPromptImmediately) {
        setPromptEditing(true);
      }

      document.getElementById("btn-auto-back")?.addEventListener("click", () => {
        if (isPromptDirty()) {
          const msg = typeof t === "function" ? t("有未保存的指令修改，确定放弃并返回吗？") : "有未保存的指令修改，确定放弃并返回吗？";
          if (!confirm(msg)) return;
        }
        this._selectedAutomationId = null;
        this.renderToolsTab(false);
      });

      // Inline Title Editor
      const titleView = document.getElementById("p3-auto-title-view");
      const titleEditor = document.getElementById("p3-auto-title-editor");
      document.getElementById("btn-auto-title-edit")?.addEventListener("click", () => {
        titleView?.classList.add("hidden");
        titleEditor?.classList.remove("hidden");
        document.getElementById("p3-edit-auto-name")?.focus();
      });
      document.getElementById("btn-auto-title-cancel")?.addEventListener("click", () => {
        titleEditor?.classList.add("hidden");
        titleView?.classList.remove("hidden");
      });
      document.getElementById("btn-auto-title-save")?.addEventListener("click", async () => {
        const name = document.getElementById("p3-edit-auto-name")?.value.trim() || "";
        if (!name) return;
        const r = await fetch("/gateway/automations/update", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ sidecarId: sc.id, name }),
        });
        const d = await r.json().catch(() => ({}));
        if (!r.ok || d.ok === false) {
          showToast?.("保存失败: " + (d.error || r.status));
          return;
        }
        showToast?.("已更新任务名称");
        this.renderToolsTab(false);
      });

      // Config Summary Card Toggle & Save
      const configEditor = document.getElementById("p3-auto-config-editor");
      const toggleConfigEditor = (open) => {
        const nextOpen = typeof open === "boolean" ? open : configEditor?.classList.contains("hidden");
        configEditor?.classList.toggle("hidden", !nextOpen);
        if (nextOpen && !schedMounted) {
          schedMounted = true;
          this._mountAutoScheduleEditor(document.getElementById("p3-edit-auto-sched-mount"), "edit", schedDraft);
        }
      };
      document.getElementById("btn-auto-config-toggle")?.addEventListener("click", () => toggleConfigEditor());
      document.getElementById("btn-auto-config-cancel")?.addEventListener("click", () => toggleConfigEditor(false));
      document.getElementById("btn-auto-config-save")?.addEventListener("click", async () => {
        const cron = this._buildAutoCron(schedDraft).trim();
        const model = document.getElementById("p3-edit-auto-model")?.value || "";
        const projectId = document.getElementById("p3-edit-auto-project")?.value || "";
        if (!cron) {
          showToast?.("请填写有效的 Cron 表达式");
          return;
        }
        const r = await fetch("/gateway/automations/update", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ sidecarId: sc.id, cron, model, projectId }),
        });
        const d = await r.json().catch(() => ({}));
        if (!r.ok || d.ok === false) {
          showToast?.("保存失败: " + (d.error || r.status));
          return;
        }
        showToast?.("已保存配置");
        this.renderToolsTab(false);
      });

      // Prompt Markdown Expand & Edit
      document.getElementById("btn-auto-prompt-expand")?.addEventListener("click", (e) => {
        const mdEl = document.getElementById("p3-auto-prompt-md");
        if (!mdEl) return;
        const expanded = !mdEl.classList.toggle("collapsed");
        e.target.textContent = expanded ? (typeof t === "function" ? t("收起指令") : "收起指令") : (typeof t === "function" ? t("展开完整指令") : "展开完整指令");
      });
      document.getElementById("btn-auto-prompt-edit")?.addEventListener("click", () => setPromptEditing(true));
      document.getElementById("btn-auto-prompt-cancel")?.addEventListener("click", () => {
        if (isPromptDirty()) {
          const msg = typeof t === "function" ? t("有未保存的指令修改，确定放弃吗？") : "有未保存的指令修改，确定放弃吗？";
          if (!confirm(msg)) return;
        }
        setPromptEditing(false);
      });
      document.getElementById("btn-auto-prompt-save")?.addEventListener("click", async () => {
        const prompt = document.getElementById("p3-edit-auto-prompt")?.value.trim() || "";
        if (!prompt) {
          showToast?.("执行指令不能为空");
          return;
        }
        const btn = document.getElementById("btn-auto-prompt-save");
        if (btn) btn.disabled = true;
        const r = await fetch("/gateway/automations/update", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ sidecarId: sc.id, prompt }),
        });
        const d = await r.json().catch(() => ({}));
        if (!r.ok || d.ok === false) {
          if (btn) btn.disabled = false;
          showToast?.("保存失败: " + (d.error || r.status));
          return;
        }
        showToast?.("已保存执行指令");
        this.renderToolsTab(false);
      });

      // Lifecycle controls
      document.getElementById("p3-auto-detail-switch")?.addEventListener("change", async (e) => {
        const wantStart = !!e.target.checked;
        e.target.disabled = true;
        const r = await fetch("/gateway/automations/manage", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ sidecarId: sc.id, action: wantStart ? "start" : "stop" }),
        });
        const d = await r.json().catch(() => ({}));
        if (!r.ok) {
          e.target.checked = !wantStart;
          e.target.disabled = false;
          showToast?.("操作失败: " + (d.error || r.status));
          return;
        }
        showToast?.(wantStart ? "已启动定时任务" : "已暂停定时任务");
        this.renderToolsTab(false);
      });
      document.getElementById("btn-auto-detail-run")?.addEventListener("click", async () => {
        if (!running) return;
        const r = await fetch("/gateway/automations/manage", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ sidecarId: sc.id, action: "run_now" }),
        });
        const d = await r.json().catch(() => ({}));
        if (!r.ok) {
          showToast?.("立即运行失败: " + (d.error || r.status));
          return;
        }
        showToast?.("已触发立即执行");
        this.renderToolsTab(false);
      });
      document.getElementById("btn-auto-detail-restart")?.addEventListener("click", async () => {
        const r = await fetch("/gateway/automations/manage", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ sidecarId: sc.id, action: "restart" }),
        });
        const d = await r.json().catch(() => ({}));
        if (!r.ok) {
          showToast?.("重启失败: " + (d.error || r.status));
          return;
        }
        showToast?.("已重启任务");
        this.renderToolsTab(false);
      });
      document.getElementById("btn-auto-detail-del")?.addEventListener("click", async () => {
        if (!confirm(typeof t === "function" ? t(`确定删除定时任务「${sc.name || sc.id}」？`) : `确定删除定时任务「${sc.name || sc.id}」？`)) return;
        const r = await fetch("/gateway/automations/delete", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ sidecarId: sc.id }),
        });
        if (r.ok) {
          this._selectedAutomationId = null;
          showToast?.("已删除");
          this.renderToolsTab(false);
        }
      });

      // Multi-file Logs Viewer
      let selectedLogFile = "";
      const loadLogs = async (fileOverride) => {
        const box = document.getElementById("p3-auto-logs-box");
        const pre = document.getElementById("p3-auto-logs-pre");
        const sel = document.getElementById("p3-auto-logs-file");
        if (!box || !pre) return;
        if (typeof fileOverride === "string") selectedLogFile = fileOverride;
        box.classList.remove("hidden");
        pre.textContent = "加载中…";
        try {
          const qs = new URLSearchParams({ sidecarId: sc.id, tail: "200" });
          if (selectedLogFile) qs.set("file", selectedLogFile);
          const r = await fetch(`/gateway/automations/logs?${qs.toString()}`);
          const d = await r.json();
          const files = Array.isArray(d.files) ? d.files : [];
          if (sel && files.length > 1) {
            const activeFile = d.file || selectedLogFile || files[0];
            selectedLogFile = activeFile;
            sel.innerHTML = files.map((f) => `<option value="${escapeHtml(f)}" ${f === activeFile ? "selected" : ""}>${escapeHtml(f)}</option>`).join("");
            sel.classList.remove("hidden");
          }
          pre.textContent = (d.logs && d.logs.trim()) ? d.logs : "暂无日志输出";
        } catch (err) {
          pre.textContent = "加载日志失败: " + err.message;
        }
      };
      document.getElementById("btn-auto-detail-logs")?.addEventListener("click", () => loadLogs());
      document.getElementById("btn-auto-logs-refresh")?.addEventListener("click", () => loadLogs());
      document.getElementById("p3-auto-logs-file")?.addEventListener("change", (e) => loadLogs(e.target.value));

      body.querySelectorAll("[data-auto-conv]").forEach((row) => {
        row.addEventListener("click", () => {
          const cid = row.getAttribute("data-auto-conv");
          const exists = row.getAttribute("data-auto-conv-exists") !== "0";
          if (!exists) {
            showToast?.(typeof t === "function" ? t("该触发记录对应的会话已删除") : "该触发记录对应的会话已删除", 2400);
            return;
          }
          const openConv = typeof selectCascadeConversation === "function"
            ? selectCascadeConversation
            : (typeof selectConversation === "function" ? selectConversation : null);
          if (cid && openConv) {
            this.closeToolsBoard();
            openConv(cid);
          }
        });
      });
    },

    openSidecarSheet(title, url) {
      this.ensureOverlays();
      const sc = document.getElementById("sheet-sidecar-web");
      const ttl = document.getElementById("sidecar-web-title");
      const frame = document.getElementById("sidecar-web-frame");
      if (ttl) ttl.textContent = title || "Sidecar 界面";
      if (frame) frame.src = url;
      sc?.classList.remove("hidden");
    },

    closeSidecarSheet() {
      const sc = document.getElementById("sheet-sidecar-web");
      const frame = document.getElementById("sidecar-web-frame");
      if (frame) frame.src = "about:blank";
      sc?.classList.add("hidden");
    },

    // --- 9. Subagent Interactive Modal & Browser Screenshot/Skip/Whitelist ---
    async openSubagentDetail(sub) {
      this.ensureOverlays();
      const sd = document.getElementById("sheet-subagent-detail");
      const ttl = document.getElementById("subagent-detail-title");
      const body = document.getElementById("subagent-detail-body");
      if (!sd || !body || !sub?.conversationId) return;

      const cid = sub.conversationId;
      const isBrowser = (sub.typeName || "").toLowerCase().includes("browser");
      if (ttl) ttl.textContent = subagentDisplayName?.(sub) || "子代理协作";
      sd.classList.remove("hidden");

      let browserHtml = "";
      let browserUrlForLive = "";
      if (isBrowser) {
        let bData = {};
        try {
          const bRes = await fetch(`/gateway/subagent/browser?cascadeId=${encodeURIComponent(cid)}`);
          bData = await bRes.json().catch(() => ({}));
        } catch (_) {}
        browserUrlForLive = bData.url || "";
        const imgSrc = bData.imageBase64
          ? (bData.imageBase64.startsWith("data:") ? bData.imageBase64 : `data:image/png;base64,${bData.imageBase64}`)
          : (bData.screenshotUri ? `/gateway/artifact-raw?uri=${encodeURIComponent(bData.screenshotUri)}` : "");
        browserHtml = `
          <div class="p3-browser-box">
            ${bData.url ? `<div class="p3-row-sub monospaced">🌐 ${escapeHtml(bData.url)}</div>` : ""}
            ${imgSrc ? `<img src="${escapeHtml(imgSrc)}" class="p3-browser-shot" alt="浏览器实时画面" />` : `<div class="p3-empty">暂无浏览器截图</div>`}
            <div class="p3-form-actions">
              <button type="button" class="p3-mini-btn primary" id="btn-sa-live-browser">实时接管浏览器</button>
              ${bData.url ? `<button type="button" class="p3-mini-btn" id="btn-sa-whitelist" data-url="${escapeHtml(bData.url)}">加白名单</button>` : ""}
              <button type="button" class="p3-mini-btn danger" id="btn-sa-skip">跳过此步</button>
            </div>
          </div>`;
      }

      const rtModel = (sub.runtimeModel || "").trim();
      const reqTier = typeof subagentRequestedTier === "function" ? subagentRequestedTier(sub) : (sub.modelTier || "");
      const modelMetaHtml = rtModel
        ? ` · 实际模型: <span class="monospaced">${escapeHtml(rtModel)}</span>${reqTier ? ` (请求: ${escapeHtml(reqTier)})` : ""}`
        : (reqTier ? ` · 模型档位: <span class="monospaced">${escapeHtml(reqTier)}</span>` : "");

      body.innerHTML = `
        <div class="p3-row-sub">会话 ID: <span class="monospaced">${escapeHtml(cid)}</span> · 状态: ${escapeHtml(sub.status || "")}${modelMetaHtml}</div>
        ${browserHtml}
        <div class="p3-form-card">
          <div class="p3-row-title">向子代理中途插话 / 指示</div>
          <textarea id="p3-sa-msg" class="p3-textarea" rows="3" placeholder="向正在运行的子代理发送补充指令…"></textarea>
          <div class="p3-form-actions">
            <button type="button" class="p3-mini-btn p3-mini-btn-btw" id="btn-sa-btw">${typeof window.renderSideQuestionIconHtml === "function" ? window.renderSideQuestionIconHtml(12) : ""}<span>${typeof t === "function" ? t("/btw 旁问") : "/btw 旁问"}</span></button>
            <button type="button" class="p3-mini-btn" id="btn-sa-jump">只读查看完整对话</button>
            <button type="button" class="p3-mini-btn primary" id="btn-sa-send">发送消息</button>
          </div>
        </div>`;

      document.getElementById("btn-sa-btw")?.addEventListener("click", () => {
        sd.classList.add("hidden");
        const quote = `子代理「${subagentDisplayName?.(sub) || cid.slice(0, 8)}」(${sub.status || "running"})`;
        window.V233?.openBtwModal?.({ parentCascadeId: activeCascadeId, quote });
      });
      document.getElementById("btn-sa-jump")?.addEventListener("click", () => {
        sd.classList.add("hidden");
        navigateTo?.(`#c=${cid}`);
      });
      document.getElementById("btn-sa-send")?.addEventListener("click", async () => {
        const content = document.getElementById("p3-sa-msg")?.value.trim();
        if (!content) return;
        const r = await fetch("/gateway/subagent/message", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ recipient: cid, content }),
        });
        if (r.ok) {
          document.getElementById("p3-sa-msg").value = "";
          showToast?.("已向子代理发送指示");
        } else {
          showToast?.("发送失败");
        }
      });
      document.getElementById("btn-sa-live-browser")?.addEventListener("click", () => {
        sd.classList.add("hidden");
        window.Phase4?.openBrowserViewer?.({ initialUrl: browserUrlForLive });
      });
      document.getElementById("btn-sa-skip")?.addEventListener("click", async () => {
        await fetch("/gateway/subagent/browser/skip", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ cascadeId: cid }),
        });
        showToast?.("已请求跳过当前浏览器步骤");
      });
      document.getElementById("btn-sa-whitelist")?.addEventListener("click", async (e) => {
        const url = e.target.getAttribute("data-url");
        if (!url) return;
        await fetch("/gateway/subagent/browser/whitelist", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ url }),
        });
        showToast?.("已添加到浏览器允许名单");
      });
    },

    // --- 10. Running Task: Send to Background ---
    async sendTaskToBackground(stepIndex) {
      if (!activeCascadeId) return;
      const res = await fetch("/gateway/cascade/task/background", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ cascadeId: activeCascadeId, stepIndices: [Number(stepIndex)] }),
      });
      const d = await res.json().catch(() => ({}));
      if (d.ok) showToast?.("已转入后台执行");
      else showToast?.(d.unsupported ? "当前上游版本暂不支持步骤级转后台" : (d.message || "转后台失败"), 2600);
    },
  };

  window.Phase3 = Phase3;
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", () => Phase3.init());
  } else {
    Phase3.init();
  }
})();
