// web/js/swarm.js — Native Swarm Manager for Multigravity Web PWA (Antigravity 2.31+)

const SwarmManager = (() => {
  let activeCascadeId = null;
  let swarmBrief = null;
  let swarmSnapshot = null;
  let activeTab = "overview";
  let pollInterval = null;

  function init() {
    createSwarmUI();
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
    activeCascadeId = data.cascadeId;
    if (data.swarm && data.swarm.active) {
      swarmBrief = data.swarm;
      updateHeaderPill(true);
      if (!document.getElementById("swarm-modal")?.classList.contains("hidden")) {
        fetchSnapshot();
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
      fetchSnapshot();
      if (!pollInterval) {
        pollInterval = setInterval(fetchSnapshot, 2500);
      }
    } else {
      modal.classList.add("hidden");
      if (pollInterval) {
        clearInterval(pollInterval);
        pollInterval = null;
      }
    }
  }

  async function fetchSnapshot() {
    if (!activeCascadeId) return;
    try {
      const res = await fetch(`/gateway/swarm?cascadeId=${encodeURIComponent(activeCascadeId)}`);
      if (res.ok) {
        swarmSnapshot = await res.json();
        updateModalHeader();
        renderCurrentTab();
      }
    } catch (err) {
      console.warn("[Swarm] Failed to fetch snapshot:", err);
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

    container.innerHTML = `
      <div class="swarm-board-container">
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
        if (cid && typeof window.selectCascadeConversation === "function") {
          toggleSwarmModal(false);
          window.selectCascadeConversation(cid);
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

  return {
    init,
    sync,
    toggleSwarmModal,
  };
})();

// Auto-initialize when DOM ready
if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", SwarmManager.init);
} else {
  SwarmManager.init();
}
