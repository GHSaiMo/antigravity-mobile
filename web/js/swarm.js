// web/js/swarm.js — Native Swarm Manager for Multigravity Web PWA (Antigravity 2.31+)

const SwarmManager = (() => {
  let activeCascadeId = null;
  let swarmBrief = null;
  let swarmSnapshot = null;
  let activeTab = "overview";
  let pollInterval = null;
  let lastETag = "";
  let loadingEarlier = false;
  const SWARM_PAGE_SIZE = 80;

  function init() {
    createSwarmUI();
    window.addEventListener("hashchange", () => {
      const modal = document.getElementById("swarm-modal");
      if (modal && !modal.classList.contains("hidden")) {
        toggleSwarmModal(false);
      }
    });
  }

  function createSwarmUI() {
    // 1. Swarm Header Pill (in chat header)
    if (!document.getElementById("swarm-header-pill")) {
      const headerRight = document.querySelector(".chat-header-actions") || document.querySelector(".header-right") || document.querySelector("header");
      if (headerRight) {
        const pill = document.createElement("button");
        pill.id = "swarm-header-pill";
        pill.className = "swarm-header-pill hidden";
        pill.innerHTML = `
          <span class="swarm-dot"></span>
          <span class="swarm-pill-title">蜂群协作</span>
          <span class="swarm-pill-badge" id="swarm-pill-badge">0</span>
        `;
        pill.onclick = () => toggleSwarmModal(true);
        headerRight.prepend(pill);
      }
    }

    // 2. Swarm Modal / Drawer
    if (!document.getElementById("swarm-modal")) {
      const modal = document.createElement("div");
      modal.id = "swarm-modal";
      modal.className = "swarm-modal-overlay hidden";
      modal.innerHTML = `
        <div class="swarm-modal-dialog">
          <div class="swarm-modal-header">
            <div class="swarm-modal-title-box">
              <span class="swarm-icon">🐝</span>
              <h3 id="swarm-modal-title">蜂群看板</h3>
              <span class="swarm-phase-tag" id="swarm-phase-tag">规划中</span>
            </div>
            <button class="swarm-close-btn" id="swarm-close-btn">&times;</button>
          </div>
          <div class="swarm-tabs-nav">
            <button class="swarm-tab-btn active" data-tab="overview">综述 (Overview)</button>
            <button class="swarm-tab-btn" data-tab="board">消息板 (Board)</button>
            <button class="swarm-tab-btn" data-tab="members">成员 (<span id="swarm-tab-member-count">0</span>)</button>
            <button class="swarm-tab-btn" data-tab="graph">协作图 (Graph)</button>
          </div>
          <div class="swarm-modal-body" id="swarm-modal-content">
            <div class="swarm-loading">正在读取蜂群状态...</div>
          </div>
        </div>
      `;
      document.body.appendChild(modal);

      modal.querySelector("#swarm-close-btn").onclick = () => toggleSwarmModal(false);
      modal.onclick = (e) => {
        if (e.target === modal) toggleSwarmModal(false);
      };

      const tabs = modal.querySelectorAll(".swarm-tab-btn");
      tabs.forEach(btn => {
        btn.onclick = () => {
          tabs.forEach(t => t.classList.remove("active"));
          btn.classList.add("active");
          activeTab = btn.getAttribute("data-tab");
          renderCurrentTab();
        };
      });
    }
  }

  function sync(data) {
    if (!data) return;
    if (activeCascadeId !== data.cascadeId) {
      activeCascadeId = data.cascadeId;
      swarmSnapshot = null;
      lastETag = "";
    }
    if (data.swarm && data.swarm.active) {
      swarmBrief = data.swarm;
      updateHeaderPill(true);
      if (!document.getElementById("swarm-modal")?.classList.contains("hidden")) {
        fetchSnapshot(true);
      }
    } else {
      swarmBrief = null;
      updateHeaderPill(false);
    }
  }

  function updateHeaderPill(visible) {
    const pill = document.getElementById("swarm-header-pill");
    if (!pill) return;
    if (!visible || !swarmBrief) {
      pill.classList.add("hidden");
      return;
    }
    pill.classList.remove("hidden");
    const titleSpan = pill.querySelector(".swarm-pill-title");
    const badgeSpan = document.getElementById("swarm-pill-badge");
    if (titleSpan) {
      const phase = swarmBrief.phase ? ` · ${swarmBrief.phase}` : "";
      titleSpan.textContent = `${swarmBrief.title || "蜂群"}${phase}`;
    }
    if (badgeSpan) {
      if (swarmBrief.attentionCount > 0) {
        badgeSpan.textContent = `!${swarmBrief.attentionCount}`;
        badgeSpan.style.background = "#ef4444";
      } else {
        badgeSpan.textContent = `${swarmBrief.workingCount}/${swarmBrief.totalMembers}`;
        badgeSpan.style.background = "#8b5cf6";
      }
    }
  }

  function toggleSwarmModal(open) {
    const modal = document.getElementById("swarm-modal");
    if (!modal) return;
    if (open) {
      modal.classList.remove("hidden");
      fetchSnapshot(false);
      if (!pollInterval) {
        pollInterval = setInterval(() => fetchSnapshot(true), 2500);
      }
    } else {
      modal.classList.add("hidden");
      if (pollInterval) {
        clearInterval(pollInterval);
        pollInterval = null;
      }
    }
  }

  function mergeSwarmMessages(existing, incoming) {
    const bySeq = new Map();
    for (const m of (existing || [])) bySeq.set(m.seq, m);
    for (const m of (incoming || [])) bySeq.set(m.seq, m);
    return Array.from(bySeq.values()).sort((a, b) => a.seq - b.seq);
  }

  async function fetchSnapshot(incremental = false) {
    if (!activeCascadeId || document.visibilityState !== "visible") return;
    try {
      const params = new URLSearchParams({ cascadeId: activeCascadeId });
      const hasExisting = Boolean(incremental && swarmSnapshot && Array.isArray(swarmSnapshot.messages) && swarmSnapshot.messages.length > 0);
      if (hasExisting) {
        const latestSeq = swarmSnapshot.messages[swarmSnapshot.messages.length - 1]?.seq || 0;
        if (latestSeq > 0) params.set("afterSeq", String(latestSeq));
      } else {
        params.set("limit", String(SWARM_PAGE_SIZE));
      }
      const headers = {};
      if (hasExisting && lastETag) headers["If-None-Match"] = lastETag;
      const res = await fetch(`/gateway/swarm?${params.toString()}`, { headers });
      if (res.status === 304) return;
      if (res.ok) {
        const incoming = await res.json();
        lastETag = res.headers.get("ETag") || incoming.etag || lastETag;
        if (hasExisting) {
          const mergedMsgs = mergeSwarmMessages(swarmSnapshot.messages, incoming.messages);
          swarmSnapshot = {
            ...incoming,
            messages: mergedMsgs,
            hasMoreBefore: swarmSnapshot.hasMoreBefore,
            oldestSeq: mergedMsgs[0]?.seq || swarmSnapshot.oldestSeq || 0,
            totalMessages: Math.max(incoming.totalMessages || 0, mergedMsgs.length),
          };
        } else {
          swarmSnapshot = incoming;
        }
        updateModalHeader();
        renderCurrentTab();
      }
    } catch (err) {
      console.warn("[Swarm] Failed to fetch snapshot:", err);
    }
  }

  async function loadEarlierMessages() {
    if (!activeCascadeId || !swarmSnapshot || loadingEarlier || !swarmSnapshot.hasMoreBefore) return;
    const oldest = swarmSnapshot.oldestSeq || (swarmSnapshot.messages?.[0]?.seq || 0);
    if (!oldest) return;
    loadingEarlier = true;
    renderCurrentTab();
    try {
      const params = new URLSearchParams({
        cascadeId: activeCascadeId,
        beforeSeq: String(oldest),
        limit: String(SWARM_PAGE_SIZE),
      });
      const res = await fetch(`/gateway/swarm?${params.toString()}`);
      if (res.ok) {
        const older = await res.json();
        const mergedMsgs = mergeSwarmMessages(older.messages, swarmSnapshot.messages);
        swarmSnapshot = {
          ...swarmSnapshot,
          messages: mergedMsgs,
          hasMoreBefore: Boolean(older.hasMoreBefore),
          oldestSeq: mergedMsgs[0]?.seq || older.oldestSeq || 0,
          totalMessages: Math.max(older.totalMessages || 0, mergedMsgs.length),
        };
      }
    } catch (err) {
      console.warn("[Swarm] Failed to load earlier messages:", err);
    } finally {
      loadingEarlier = false;
      renderCurrentTab();
    }
  }

  function updateModalHeader() {
    if (!swarmSnapshot) return;
    const titleEl = document.getElementById("swarm-modal-title");
    const phaseEl = document.getElementById("swarm-phase-tag");
    const memberCountEl = document.getElementById("swarm-tab-member-count");

    if (titleEl) titleEl.textContent = swarmSnapshot.title || "蜂群协作";
    if (phaseEl) {
      phaseEl.textContent = swarmSnapshot.brief?.phase || "执行中";
    }
    if (memberCountEl) {
      memberCountEl.textContent = swarmSnapshot.members?.length || 0;
    }
  }

  function renderCurrentTab() {
    const content = document.getElementById("swarm-modal-content");
    if (!content) return;
    if (!swarmSnapshot) {
      content.innerHTML = `<div class="swarm-loading">暂无活跃蜂群数据</div>`;
      return;
    }

    switch (activeTab) {
      case "overview":
        renderOverviewTab(content);
        break;
      case "board":
        renderBoardTab(content);
        break;
      case "members":
        renderMembersTab(content);
        break;
      case "graph":
        renderGraphTab(content);
        break;
    }
  }

  function renderOverviewTab(container) {
    const view = swarmSnapshot.view;
    let headlineHtml = view?.headline
      ? `<div class="swarm-headline-card"><h4>📌 当前要点</h4><p>${escapeHtml(view.headline)}</p></div>`
      : "";

    let attentionHtml = "";
    if (view?.attention && view.attention.length > 0) {
      attentionHtml = `
        <div class="swarm-attention-section">
          <h4>⚠️ 待处理事项 (${view.attention.length})</h4>
          <div class="swarm-attention-list">
            ${view.attention.map(att => `
              <div class="swarm-attention-item severity-${escapeHtml(att.severity || 'med')}">
                <div class="swarm-att-title"><strong>${escapeHtml(att.title)}</strong> <span class="swarm-att-kind">[${escapeHtml(att.kind)}]</span></div>
                ${att.why ? `<div class="swarm-att-why">${escapeHtml(att.why)}</div>` : ""}
              </div>
            `).join("")}
          </div>
        </div>
      `;
    }

    let workstreamsHtml = "";
    if (view?.workstreams && view.workstreams.length > 0) {
      workstreamsHtml = `
        <div class="swarm-workstreams-section">
          <h4>工作流推进 (${view.workstreams.length})</h4>
          <div class="swarm-workstreams-list">
            ${view.workstreams.map(ws => `
              <div class="swarm-ws-item state-${escapeHtml(ws.state || 'active')}">
                <div class="swarm-ws-header">
                  <span class="swarm-ws-title">${escapeHtml(ws.title)}</span>
                  <span class="swarm-ws-badge">${escapeHtml(ws.state)}</span>
                </div>
                ${ws.note ? `<div class="swarm-ws-note">${escapeHtml(ws.note)}</div>` : ""}
                ${ws.owners ? `<div class="swarm-ws-owners">负责人: ${escapeHtml(ws.owners.join(', '))}</div>` : ""}
              </div>
            `).join("")}
          </div>
        </div>
      `;
    }

    container.innerHTML = `
      <div class="swarm-overview-container">
        ${headlineHtml}
        ${attentionHtml}
        ${workstreamsHtml}
        <div class="swarm-feedback-box">
          <h4>向协调员 / Narrator 纠偏反馈</h4>
          <div class="swarm-feedback-input-row">
            <input type="text" id="swarm-feedback-input" placeholder="输入反馈（例如：先聚焦完成登录模块...）" />
            <button id="swarm-feedback-submit-btn">发送</button>
          </div>
        </div>
      </div>
    `;

    const sendBtn = document.getElementById("swarm-feedback-submit-btn");
    const inputEl = document.getElementById("swarm-feedback-input");
    if (sendBtn && inputEl) {
      sendBtn.onclick = async () => {
        const text = inputEl.value.trim();
        if (!text) return;
        sendBtn.disabled = true;
        sendBtn.textContent = "发送中...";
        try {
          const res = await fetch("/gateway/swarm/feedback", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
              cascadeId: activeCascadeId,
              coordinatorId: swarmSnapshot.coordinatorId || swarmSnapshot.coordinatorID,
              feedback: text,
            }),
          });
          if (res.ok) {
            inputEl.value = "";
            alert("已成功向协调员同步反馈！");
          } else {
            alert("发送失败，请稍后再试");
          }
        } catch (err) {
          alert("请求出错: " + err.message);
        } finally {
          sendBtn.disabled = false;
          sendBtn.textContent = "发送";
        }
      };
    }
  }

  function renderBoardTab(container) {
    const msgs = swarmSnapshot.messages || [];
    if (msgs.length === 0) {
      container.innerHTML = `<div class="swarm-empty">消息板暂无内容</div>`;
      return;
    }

    const loadMoreHtml = swarmSnapshot.hasMoreBefore
      ? `<div style="text-align:center;padding:8px 0 12px;"><button id="swarm-load-earlier-btn" class="swarm-nudge-btn" ${loadingEarlier ? "disabled" : ""}>${loadingEarlier ? "正在加载更早消息..." : "加载更早消息"}</button></div>`
      : "";

    container.innerHTML = `
      <div class="swarm-board-container">
        ${loadMoreHtml}
        ${msgs.map(m => {
          let text = escapeHtml(m.text || "");
          // Highlight @mentions
          text = text.replace(/@([a-zA-Z0-9_\-]+)/g, '<span class="swarm-mention-pill">@$1</span>');
          return `
            <div class="swarm-board-msg">
              <div class="swarm-msg-meta">
                <span class="swarm-msg-agent">${escapeHtml(m.agent)}</span>
                <span class="swarm-msg-seq">#${m.seq}</span>
                <span class="swarm-msg-time">${m.time ? formatTime(m.time) : ""}</span>
              </div>
              <div class="swarm-msg-text">${text}</div>
            </div>
          `;
        }).join("")}
      </div>
    `;
    const earlierBtn = document.getElementById("swarm-load-earlier-btn");
    if (earlierBtn) {
      earlierBtn.onclick = () => loadEarlierMessages();
    }
  }

  function renderMembersTab(container) {
    const members = swarmSnapshot.members || [];
    if (members.length === 0) {
      container.innerHTML = `<div class="swarm-empty">暂无成员信息</div>`;
      return;
    }

    container.innerHTML = `
      <div class="swarm-members-container">
        ${members.map(m => {
          const isError = m.status === "error";
          const statusClass = m.killed ? "status-killed" : isError ? "status-error" : (m.status === "working" ? "status-working" : "status-waiting");
          const statusLabel = m.killed ? "已终止" : isError ? "报错/卡住" : (m.status === "working" ? "工作中" : "空闲/等待");
          return `
            <div class="swarm-member-card ${m.isCurrent ? 'is-current' : ''}">
              <div class="swarm-member-header">
                <div class="swarm-member-name-row">
                  <span class="swarm-member-status-dot ${statusClass}"></span>
                  <strong>${escapeHtml(m.name)}</strong>
                  ${m.isCurrent ? '<span class="swarm-current-tag">当前会话</span>' : ''}
                  <span class="swarm-role-tag">${escapeHtml(m.role || 'worker')}</span>
                </div>
                <div class="swarm-member-actions">
                  <button class="swarm-nudge-btn" data-worker="${escapeHtml(m.name)}" title="催促 / 唤醒该智能体">⚡ 催促唤醒</button>
                  ${(m.conversationId || m.conversationID) && !m.isCurrent ? `<button class="swarm-jump-btn" data-conv="${escapeHtml(m.conversationId || m.conversationID)}">跳转查看</button>` : ''}
                </div>
              </div>
              <div class="swarm-member-body">
                <div class="swarm-member-model">模型: ${escapeHtml(m.model || '默认')}</div>
                ${m.lastActivity ? `<div class="swarm-member-activity">动态: ${escapeHtml(m.lastActivity)}</div>` : ''}
              </div>
            </div>
          `;
        }).join("")}
      </div>
    `;

    // Bind nudge buttons
    container.querySelectorAll(".swarm-nudge-btn").forEach(btn => {
      btn.onclick = async () => {
        const worker = btn.getAttribute("data-worker");
        btn.disabled = true;
        btn.textContent = "已催促";
        try {
          await fetch("/gateway/swarm/nudge", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
              cascadeId: activeCascadeId,
              coordinatorId: swarmSnapshot.coordinatorId || swarmSnapshot.coordinatorID,
              workerName: worker,
            }),
          });
        } catch (err) {
          console.warn("Nudge error:", err);
        } finally {
          setTimeout(() => {
            btn.disabled = false;
            btn.textContent = "⚡ 催促唤醒";
          }, 3000);
        }
      };
    });

    // Bind jump buttons
    container.querySelectorAll(".swarm-jump-btn").forEach(btn => {
      btn.onclick = () => {
        const cid = btn.getAttribute("data-conv");
        if (cid) {
          toggleSwarmModal(false);
          if (typeof navigateTo === "function") {
            navigateTo("#c=" + cid);
          } else if (typeof window.selectCascadeConversation === "function") {
            window.selectCascadeConversation(cid);
          }
        }
      };
    });
  }

  function renderGraphTab(container) {
    const members = swarmSnapshot.members || [];
    const msgs = swarmSnapshot.messages || [];

    // Count mentions between agents
    const mentionCounts = {};
    msgs.forEach(m => {
      if (m.mentions) {
        m.mentions.forEach(target => {
          const key = `${m.agent} -> ${target}`;
          mentionCounts[key] = (mentionCounts[key] || 0) + 1;
        })
      }
    });

    container.innerHTML = `
      <div class="swarm-graph-container">
        <h4>协作提及关系拓扑</h4>
        <div class="swarm-edges-list">
          ${Object.entries(mentionCounts).length > 0 ? Object.entries(mentionCounts).map(([edge, count]) => `
            <div class="swarm-edge-row">
              <span class="swarm-edge-label">${escapeHtml(edge)}</span>
              <span class="swarm-edge-count">${count} 次协作交流</span>
            </div>
          `).join("") : '<div class="swarm-empty">暂无提及记录</div>'}
        </div>
      </div>
    `;
  }

  function formatTime(isoStr) {
    try {
      const d = new Date(isoStr);
      return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
    } catch {
      return isoStr;
    }
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

  const pixelBeeCache = new Map();

  function shiftHue(h, target, amount) {
    const d = (((target - h + 540) % 360) + 360) % 360 - 180;
    return h + d * amount;
  }

  function hslToRGBA(rawH, rawS, rawL, a = 255) {
    const h = ((rawH % 360) + 360) % 360;
    const s = rawS / 100;
    const l = rawL / 100;
    const amp = s * Math.min(l, 1 - l);
    const comp = (n) => {
      const k = (n + h / 30) % 12;
      const v = l - amp * Math.max(-1, Math.min(k - 3, 9 - k, 1));
      return Math.max(0, Math.min(255, Math.round(v * 255)));
    };
    return [comp(0), comp(8), comp(4), a];
  }

  function drawQueenBeeFrame(ctx, offsetX, landed, frame) {
    const w = 22, h = 18;
    const img = ctx.createImageData(w, h);
    const data = img.data;
    const setPixel = (x, y, col) => {
      if (x < 0 || y < 0 || x >= w || y >= h) return;
      const idx = (y * w + x) * 4;
      const sa = col[3] / 255;
      if (col[3] === 255) {
        data[idx] = col[0];
        data[idx + 1] = col[1];
        data[idx + 2] = col[2];
        data[idx + 3] = 255;
      } else if (col[3] > 0) {
        const da = data[idx + 3] / 255;
        const outA = sa + da * (1 - sa);
        if (outA <= 0) return;
        data[idx] = Math.round((col[0] * sa + data[idx] * da * (1 - sa)) / outA);
        data[idx + 1] = Math.round((col[1] * sa + data[idx + 1] * da * (1 - sa)) / outA);
        data[idx + 2] = Math.round((col[2] * sa + data[idx + 2] * da * (1 - sa)) / outA);
        data[idx + 3] = Math.round(outA * 255);
      }
    };

    const hue = 46;
    const r = {
      spec: hslToRGBA(shiftHue(hue, 55, 0.28), 96, 90),
      hi: hslToRGBA(shiftHue(hue, 55, 0.18), 90, 74),
      mid: hslToRGBA(hue, 78, 60),
      sh: hslToRGBA(shiftHue(hue, 220, 0.22), 58, 44),
      ol: hslToRGBA(shiftHue(hue, 220, 0.35), 42, 19),
      skh: hslToRGBA(shiftHue(hue, 220, 0.15), 20, 30),
      sk: hslToRGBA(shiftHue(hue, 220, 0.30), 24, 18),
      skd: hslToRGBA(shiftHue(hue, 220, 0.45), 32, 10),
    };

    const cx = 11.0, cy = 10.5, rx = 7.3, ry = 4.8;
    const inside = (x, y) => {
      const nx = (x + 0.5 - cx) / rx;
      const ny = (y + 0.5 - cy) / ry;
      return nx * nx + ny * ny <= 1.0;
    };
    const lenL = Math.hypot(-0.42, -0.62, 0.66);
    const lx = -0.42 / lenL, ly = -0.62 / lenL, lz = 0.66 / lenL;
    const n4 = [[1, 0], [-1, 0], [0, 1], [0, -1]];
    const rows = (spec) => {
      const pts = [];
      for (const [rowY, x0, x1] of spec) {
        for (let x = x0; x <= x1; x++) pts.push([x, rowY]);
      }
      return pts;
    };
    const drawWing = (pts, glint) => {
      const set = new Set(pts.map(([x, y]) => `${x},${y}`));
      const edgeCol = [140, 160, 192, 230];
      const fillCol = [255, 255, 255, 184];
      for (const [x, y] of pts) {
        const isEdge = n4.some(([dx, dy]) => !set.has(`${x + dx},${y + dy}`));
        setPixel(x, y, isEdge ? edgeCol : fillCol);
      }
      if (glint) {
        const interior = pts
          .filter(([x, y]) => !n4.some(([dx, dy]) => !set.has(`${x + dx},${y + dy}`)))
          .sort((a, b) => a[1] - b[1] || a[0] - b[0]);
        if (interior.length) setPixel(interior[0][0], interior[0][1], [255, 255, 255, 255]);
      }
    };

    let backWing = null, frontWing;
    if (landed) {
      frontWing = rows([[5, 14, 17], [6, 13, 18]]);
    } else if (frame === 0) {
      backWing = rows([[1, 16, 17], [2, 16, 18], [3, 15, 18], [4, 15, 17], [5, 15, 16]]);
      frontWing = rows([[0, 13, 14], [1, 12, 15], [2, 11, 15], [3, 11, 14], [4, 11, 13], [5, 11, 12]]);
    } else {
      backWing = rows([[3, 16, 18], [4, 15, 19], [5, 16, 18]]);
      frontWing = rows([[3, 13, 16], [4, 12, 17], [5, 12, 17], [6, 13, 15]]);
    }
    if (backWing) drawWing(backWing, false);

    const stripe = (x, y) => {
      const ny = (y + 0.5 - cy) / ry;
      const u = x + 0.5 + 1.15 * ny * ny;
      return (u > 10.6 && u < 12.5) || (u > 14.3 && u < 16.1) || u > 17.9;
    };

    for (let y = 0; y < h; y++) {
      for (let x = 0; x < w; x++) {
        if (!inside(x, y)) continue;
        const nx = (x + 0.5 - cx) / rx;
        const ny = (y + 0.5 - cy) / ry;
        const nz = Math.sqrt(Math.max(0, 1 - Math.min(1, nx * nx + ny * ny)));
        const intensity = nx * lx + ny * ly + nz * lz;
        if (stripe(x, y)) {
          setPixel(x, y, intensity > 0.80 ? r.skh : intensity > 0.30 ? r.sk : r.skd);
        } else {
          setPixel(x, y, intensity > 0.965 ? r.spec : intensity > 0.78 ? r.hi : intensity > 0.26 ? r.mid : r.sh);
        }
      }
    }

    for (let y = 0; y < h; y++) {
      for (let x = 0; x < w; x++) {
        if (inside(x, y)) continue;
        const nbs = n4.filter(([dx, dy]) => inside(x + dx, y + dy));
        if (!nbs.length) continue;
        const nx = (x + 0.5 - cx) / rx;
        const ny = (y + 0.5 - cy) / ry;
        const n = Math.hypot(nx, ny);
        const d = (nx / n) * lx + (ny / n) * ly;
        const nextToStripe = nbs.some(([dx, dy]) => stripe(x + dx, y + dy));
        setPixel(x, y, d > 0.5 && !nextToStripe ? r.sh : r.ol);
      }
    }

    // Stinger, eyes, blush, queen crown
    setPixel(19, 10, r.ol);
    setPixel(20, 10, r.ol);
    setPixel(6, 9, [255, 255, 255, 255]);
    setPixel(7, 9, r.skd);
    setPixel(6, 10, r.skd);
    setPixel(7, 10, r.skd);
    setPixel(5, 11, [245, 132, 96, 140]);
    setPixel(4, 11, [245, 132, 96, 90]);

    const g1 = [247, 206, 91, 255], g2 = [255, 233, 166, 255], g3 = [201, 138, 43, 255], gem = [228, 100, 62, 255];
    for (const [cx2, cy2] of [[5, 5], [6, 5], [7, 5], [8, 5], [9, 5]]) setPixel(cx2, cy2, g1);
    setPixel(5, 4, g2);
    setPixel(7, 4, g2);
    setPixel(9, 4, g2);
    setPixel(7, 3, g2);
    setPixel(5, 5, g3);
    setPixel(9, 5, g3);
    setPixel(7, 5, gem);

    drawWing(frontWing, true);
    ctx.putImageData(img, offsetX, 0);
  }

  function getQueenBeeSpriteSheetURL(isWorking) {
    const key = isWorking ? "working" : "landed";
    if (pixelBeeCache.has(key)) return pixelBeeCache.get(key);
    if (typeof document === "undefined") return "";
    const canvas = document.createElement("canvas");
    canvas.width = isWorking ? 44 : 22;
    canvas.height = 18;
    const ctx = canvas.getContext("2d");
    if (!ctx) return "";
    if (isWorking) {
      drawQueenBeeFrame(ctx, 0, false, 0);
      drawQueenBeeFrame(ctx, 22, false, 1);
    } else {
      drawQueenBeeFrame(ctx, 0, true, 0);
    }
    const url = canvas.toDataURL("image/png");
    pixelBeeCache.set(key, url);
    return url;
  }

  function renderListBeeBadge(isWorking) {
    const url = getQueenBeeSpriteSheetURL(Boolean(isWorking));
    if (!url) return "";
    const label = typeof t === "function" ? t("蜂群会话") : "蜂群会话";
    return `<span class="conv-swarm-bee${isWorking ? " is-working" : ""}" style="background-image:url('${url}')" title="${escapeHtml(label)}" aria-label="${escapeHtml(label)}"></span>`;
  }

  return {
    init,
    sync,
    toggleSwarmModal,
    renderListBeeBadge,
  };
})();

// Auto-initialize when DOM ready
if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", SwarmManager.init);
} else {
  SwarmManager.init();
}
