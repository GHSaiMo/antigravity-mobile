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

    async setPermissionPreset(preset) {
      try {
        const res = await fetch("/gateway/permissions", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ preset }),
        });
        const st = await res.json();
        if (!res.ok || st.ok === false) throw new Error(st.error || "切换失败");
        this.applyPermissionState(st);
        showToast?.(`权限档位已切换为「${st.label}」`);
      } catch (err) {
        showToast?.("切换权限失败: " + err.message, 2800);
      }
    },

    applyPermissionState(st) {
      if (!st) return;
      this.permissionState = st;
      const dot = document.getElementById("btn-tools-perm-dot");
      if (dot) dot.classList.toggle("hidden", !st.isUnlocked);
      const toolsBtn = document.getElementById("btn-chat-tools");
      if (toolsBtn) {
        toolsBtn.title = `工具与任务 (${st.label || "严格审批"}，长按/右键快切权限)`;
      }
      const sel = document.getElementById("settings-permission-preset");
      if (sel && st.preset) sel.value = st.preset;
    },

    bindPermissionsSettings() {
      const sel = document.getElementById("settings-permission-preset");
      if (sel) {
        sel.addEventListener("change", (e) => {
          this.setPermissionPreset(e.target.value);
        });
      }
      const legacyDictate = document.getElementById("settings-legacy-voice-longpress");
      if (legacyDictate) {
        legacyDictate.checked = localStorage.getItem("agy_voice_longpress_legacy") === "true";
        legacyDictate.addEventListener("change", (e) => {
          localStorage.setItem("agy_voice_longpress_legacy", e.target.checked ? "true" : "false");
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
        const cur = this.permissionState?.preset || "strict";
        const presets = [
          { id: "strict", title: "严格审批", sub: "命令/非工作区文件每次确认" },
          { id: "sandbox", title: "离席沙箱放行", sub: "沙箱内安全命令自动执行" },
          { id: "auto", title: "全自动 (Turbo)", sub: "允许自动执行与跨区文件写入" },
        ];
        menu.innerHTML = `<div class="p3-pop-title">Agent 执行权限预设</div>` + presets.map((p) => `
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
              <label class="wf-switch"><input type="checkbox" data-mcp-toggle="${escapeHtml(name)}" ${enabled ? "checked" : ""}><span class="wf-switch-slider"></span></label>
            </div>`;
          }).join("") + `</div>`;
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
              if (!item || !confirm(`确定删除「${item.title}」？`)) return;
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
          body.innerHTML = `<div class="p3-list">` + (list.map((pl) => `
            <div class="p3-row">
              <div class="p3-row-main">
                <div class="p3-row-title">${escapeHtml(pl.name || pl.id || "")}</div>
                <div class="p3-row-sub">${escapeHtml(pl.description || pl.version || "")}</div>
              </div>
              <span class="p3-badge">${pl.enabled === false ? "停用" : "启用"}</span>
            </div>`).join("") || `<div class="p3-empty">暂无已安装插件</div>`) + `</div>`;
          return;
        }

        if (this.toolsTab === "automations") {
          const res = await fetch("/gateway/automations");
          const data = await res.json();
          const list = Array.isArray(data.schedules) ? data.schedules : (Array.isArray(data.automations) ? data.automations : []);
          body.innerHTML = `
            <div class="p3-section-header">
              <span>定时自动化任务</span>
              <button type="button" class="p3-mini-btn primary" id="btn-auto-new">+ 新建定时任务</button>
            </div>
            <div id="p3-auto-form" class="p3-form-card hidden">
              <input type="text" id="p3-auto-name" class="p3-input" placeholder="任务名称 (如 晨间代码巡检)" />
              <input type="text" id="p3-auto-cron" class="p3-input monospaced" placeholder="Cron (如 0 9 * * *)" />
              <textarea id="p3-auto-prompt" class="p3-textarea" rows="3" placeholder="触发时交给 Agent 的指令…"></textarea>
              <div class="p3-form-actions">
                <button type="button" class="p3-mini-btn" id="btn-auto-cancel">取消</button>
                <button type="button" class="p3-mini-btn primary" id="btn-auto-save">创建并启用</button>
              </div>
            </div>
            <div class="p3-list">` + (list.map((sc) => `
              <div class="p3-row">
                <div class="p3-row-main">
                  <div class="p3-row-title">${escapeHtml(sc.name || sc.id)} <span class="p3-badge">${escapeHtml(sc.cronExpression || "")}</span></div>
                  <div class="p3-row-sub">${escapeHtml((sc.prompt || sc.description || "").slice(0, 80))}</div>
                </div>
                <div class="p3-row-actions">
                  <button type="button" class="p3-mini-btn" data-auto-run="${escapeHtml(sc.id)}">立即运行</button>
                  <button type="button" class="p3-mini-btn danger" data-auto-del="${escapeHtml(sc.id)}">删除</button>
                </div>
              </div>`).join("") || `<div class="p3-empty">暂无定时任务</div>`) + `</div>`;

          document.getElementById("btn-auto-new")?.addEventListener("click", () => {
            document.getElementById("p3-auto-form")?.classList.remove("hidden");
          });
          document.getElementById("btn-auto-cancel")?.addEventListener("click", () => {
            document.getElementById("p3-auto-form")?.classList.add("hidden");
          });
          document.getElementById("btn-auto-save")?.addEventListener("click", async () => {
            const name = document.getElementById("p3-auto-name").value.trim();
            const cron = document.getElementById("p3-auto-cron").value.trim();
            const prompt = document.getElementById("p3-auto-prompt").value.trim();
            const r = await fetch("/gateway/automations/create", {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ name, cron, prompt, enabled: true }),
            });
            const d = await r.json().catch(() => ({}));
            if (!r.ok || d.ok === false) {
              showToast?.("创建失败: " + (d.error || r.status));
              return;
            }
            showToast?.("已创建定时任务");
            this.renderToolsTab(false);
          });
          body.querySelectorAll("[data-auto-run]").forEach((btn) => {
            btn.addEventListener("click", async () => {
              const sidecarId = btn.getAttribute("data-auto-run");
              await fetch("/gateway/automations/manage", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ sidecarId, action: "run" }),
              });
              showToast?.("已触发立即执行");
            });
          });
          body.querySelectorAll("[data-auto-del]").forEach((btn) => {
            btn.addEventListener("click", async () => {
              const sidecarId = btn.getAttribute("data-auto-del");
              if (!confirm(`确定删除定时任务「${sidecarId}」？`)) return;
              const r = await fetch("/gateway/automations/delete", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ sidecarId }),
              });
              if (r.ok) {
                showToast?.("已删除");
                this.renderToolsTab(false);
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

    openTerminalStream(terminalId) {
      if (!terminalId) return;
      this.activeTerminalId = terminalId;
      document.getElementById("p3-terminal-drawer")?.classList.remove("hidden");
      const label = document.getElementById("p3-term-active-label");
      if (label) label.textContent = `终端: ${terminalId}`;
      if (this.activeTerminalWs) {
        try { this.activeTerminalWs.close(); } catch (_) {}
      }
      const proto = location.protocol === "https:" ? "wss:" : "ws:";
      const ws = new WebSocket(`${proto}//${location.host}/gateway/terminal/stream?terminalId=${encodeURIComponent(terminalId)}`);
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

    // --- 3. New Conversation Sheet: Worktree / Workspace Alias + GitHub Clone ---
    bindNewSheetExtras() {
      const cloneBtn = document.getElementById("btn-clone-repo");
      cloneBtn?.addEventListener("click", async () => {
        const slug = prompt("输入要克隆的 GitHub 仓库 (owner/repo 或 HTTPS URL)：");
        if (!slug || !slug.trim()) return;
        showToast?.("正在克隆仓库并初始化会话…", 6000);
        try {
          const res = await fetch("/gateway/projects/clone", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ repoSlug: slug.trim(), startSession: true }),
          });
          const d = await res.json();
          if (!res.ok || !d.ok) throw new Error(d.error || "克隆失败");
          document.getElementById("sheet-new")?.classList.add("hidden");
          if (d.cascadeId) navigateTo?.("#c=" + d.cascadeId);
          else showToast?.(`已克隆至 ${d.path}`);
        } catch (err) {
          showToast?.("克隆失败: " + err.message, 3200);
        }
      });

      const listEl = document.getElementById("new-projects-list");
      if (!listEl) return;
      let lpTimer = 0;
      let suppressNextClick = false;

      const showProjectMenu = (card) => {
        const idx = Number(card.getAttribute("data-index"));
        const proj = Array.isArray(discoveredProjects) ? discoveredProjects[idx] : null;
        if (!proj) return;
        const action = prompt(
          `工作区「${proj.name}」操作：\n1 = 在独立 Git Worktree 开启新会话\n2 = 设置工作区备注别名\n请输入序号 (1 或 2)：`
        );
        if (action === "1") {
          this.createWorktreeSession(proj);
        } else if (action === "2") {
          const cur = this.workspaceNotes[proj.uri || proj.path] || "";
          const note = prompt(`为「${proj.name}」设置备注 (留空清除)：`, cur);
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
          const val = inputEl.value;
          const m = val.match(/(?:^|\s)@([^\s@]*)$/);
          if (m) {
            this.openMentionPicker(m[1]);
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
      this.mentionOpen = false;
      document.getElementById("context-mention-picker")?.classList.add("hidden");
    },

    renderContextChips() {
      const bar = document.getElementById("context-chips-bar");
      if (!bar) return;
      if (this.contextItems.length === 0) {
        bar.classList.add("hidden");
        bar.innerHTML = "";
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
    },

    consumeContextItems() {
      const items = [...this.contextItems];
      this.contextItems = [];
      this.renderContextChips();
      return items;
    },

    // --- 5. Suggested Responses (hides when input focused) ---
    syncStreamData(data) {
      if (!data) return;
      const isRunning = data.status === "CASCADE_RUN_STATUS_RUNNING";
      this.suggestedResponses = !isRunning && Array.isArray(data.suggestedResponses) ? data.suggestedResponses : [];
      this.renderSuggestedResponses();
    },

    bindSuggestedResponsesVisibility() {
      const inputEl = document.getElementById("chat-input");
      if (!inputEl) return;
      inputEl.addEventListener("focus", () => this.renderSuggestedResponses());
      inputEl.addEventListener("blur", () => setTimeout(() => this.renderSuggestedResponses(), 120));
    },

    renderSuggestedResponses() {
      const bar = document.getElementById("suggested-responses-bar");
      if (!bar) return;
      const inputEl = document.getElementById("chat-input");
      const focused = document.activeElement === inputEl;
      if (focused || this.suggestedResponses.length === 0) {
        bar.classList.add("hidden");
        return;
      }
      bar.classList.remove("hidden");
      bar.innerHTML = this.suggestedResponses.map((text, idx) => `
        <button type="button" class="p3-suggest-pill" data-suggest-idx="${idx}">${escapeHtml(text)}</button>`).join("");
      bar.querySelectorAll("[data-suggest-idx]").forEach((btn) => {
        btn.addEventListener("click", () => {
          const text = this.suggestedResponses[Number(btn.getAttribute("data-suggest-idx"))];
          if (!text || !inputEl) return;
          inputEl.value = text;
          this.suggestedResponses = [];
          this.renderSuggestedResponses();
          sendMessage?.();
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
        if (localStorage.getItem("agy_voice_longpress_legacy") === "true") return false;
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
        this.dictateStream = stream;
        const proto = location.protocol === "https:" ? "wss:" : "ws:";
        const ws = new WebSocket(`${proto}//${location.host}/gateway/audio/dictate?cascadeId=${encodeURIComponent(activeCascadeId || "")}&mimeType=${encodeURIComponent("audio/pcm;rate=16000")}`);
        this.dictateWs = ws;

        const inputEl = document.getElementById("chat-input");
        const baseText = inputEl ? inputEl.value : "";

        ws.onmessage = (ev) => {
          try {
            const msg = JSON.parse(ev.data);
            const txt = msg.text || msg.transcript || msg.update?.text || msg.update?.transcript || "";
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
      const useWt = confirm("从该步骤分叉新会话？\n点击「确定」将在独立 Git Worktree 创建分叉（失败可取消改普通分叉）；或请用 Worktree 参数。");
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
        if (title) title.textContent = "🔊 正在语音播报";
        document.getElementById("btn-narr-toggle").textContent = "暂停";
        audio.onended = () => {
          URL.revokeObjectURL(url);
          this.stopNarration();
        };
        await audio.play();
      } catch (_) {
        // Browser speechSynthesis fallback
        if (typeof speechSynthesis !== "undefined") {
          const u = new SpeechSynthesisUtterance(cleaned);
          u.lang = "zh-CN";
          if (title) title.textContent = "🔊 系统朗读中";
          u.onend = () => this.stopNarration();
          speechSynthesis.speak(u);
        } else {
          showToast?.("当前浏览器不支持语音播报");
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
      window.addEventListener("message", async (e) => {
        const d = e.data;
        if (!d || typeof d !== "object" || !d.type) return;
        if (d.type === "sendMessage" && d.text && activeCascadeId) {
          await rpc?.("SendUserCascadeMessage", {
            cascadeId: activeCascadeId,
            items: [{ text: String(d.text) }],
            model: activeModel || undefined,
          });
          e.source?.postMessage?.({ id: d.id, ok: true }, "*");
        } else if (d.type === "updateModelContext" && d.context && activeCascadeId) {
          this.contextItems.push({
            id: "embed-" + Date.now(),
            type: "file",
            label: d.label || "交互上下文",
            scopeItem: { codeContext: d.context },
          });
          this.renderContextChips();
          e.source?.postMessage?.({ id: d.id, ok: true }, "*");
        } else if (d.type === "callServerTool" && d.toolName) {
          try {
            const res = await fetch("/gateway/mcp/app/call", {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ toolName: d.toolName, arguments: d.arguments || {}, appUri: d.appUri || "" }),
            });
            const payload = await res.json().catch(() => ({}));
            e.source?.postMessage?.({ id: d.id, ok: res.ok, result: payload }, "*");
          } catch (err) {
            e.source?.postMessage?.({ id: d.id, ok: false, error: err.message }, "*");
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
        if (!sid) return;
        this.openSidecarSheet(sid, `/gateway/sidecar-ui/${encodeURIComponent(sid)}${sub}`);
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
      if (isBrowser) {
        try {
          const bRes = await fetch(`/gateway/subagent/browser?cascadeId=${encodeURIComponent(cid)}`);
          const bData = await bRes.json();
          const imgSrc = bData.imageBase64
            ? (bData.imageBase64.startsWith("data:") ? bData.imageBase64 : `data:image/png;base64,${bData.imageBase64}`)
            : (bData.screenshotUri ? `/gateway/artifact-raw?uri=${encodeURIComponent(bData.screenshotUri)}` : "");
          browserHtml = `
            <div class="p3-browser-box">
              ${bData.url ? `<div class="p3-row-sub monospaced">🌐 ${escapeHtml(bData.url)}</div>` : ""}
              ${imgSrc ? `<img src="${escapeHtml(imgSrc)}" class="p3-browser-shot" alt="浏览器实时画面" />` : `<div class="p3-empty">暂无浏览器截图</div>`}
              <div class="p3-form-actions">
                ${bData.url ? `<button type="button" class="p3-mini-btn" id="btn-sa-whitelist" data-url="${escapeHtml(bData.url)}">加白名单</button>` : ""}
                <button type="button" class="p3-mini-btn danger" id="btn-sa-skip">跳过此步</button>
              </div>
            </div>`;
        } catch (_) {}
      }

      body.innerHTML = `
        <div class="p3-row-sub">会话 ID: <span class="monospaced">${escapeHtml(cid)}</span> · 状态: ${escapeHtml(sub.status || "")}</div>
        ${browserHtml}
        <div class="p3-form-card">
          <div class="p3-row-title">向子代理中途插话 / 指示</div>
          <textarea id="p3-sa-msg" class="p3-textarea" rows="3" placeholder="向正在运行的子代理发送补充指令…"></textarea>
          <div class="p3-form-actions">
            <button type="button" class="p3-mini-btn" id="btn-sa-jump">只读查看完整对话</button>
            <button type="button" class="p3-mini-btn primary" id="btn-sa-send">发送消息</button>
          </div>
        </div>`;

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
