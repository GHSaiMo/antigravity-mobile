// 子代理可见性。
//
// 父会话每次 invoke_subagent 在步骤流里产生一个 INVOKE_SUBAGENT 步骤，其 results[i].conversationId 指向子会话
// （子会话本身就是普通会话）。运行状态不在父会话里，由网关从会话列表快照补全后随流下发（stream 的 subagents[]）。
//
//   - 消息流里：在调用位置为每个子代理显示一张卡片（状态图标 / 角色 / 类型），点击进入子会话（只读）
//   - 底部浮层：只列仍在运行的子代理，可单独关停；结束或被关停后自动收起，不占位
//   - 子会话：只读（隐藏输入栏，换成提示条），返回回到父会话
// 关停走 POST /gateway/subagent/stop，网关只接受子代理会话，不会误停主会话。

const SubagentManager = {
  list: [], // 网关补全后的子代理（含 status / stepCount）
  byId: new Map(),
  parentId: "", // 非空说明当前会话本身是子代理
  role: "",

  // 用 stream / messages 响应里的 subagents、parentConversationId、subagentRole 同步状态
  sync(data) {
    const list = data && Array.isArray(data.subagents) ? data.subagents : [];
    this.list = list;
    this.byId = new Map(list.map((s) => [s.conversationId, s]));
    this.parentId = (data && typeof data.parentConversationId === "string" ? data.parentConversationId : "").trim();
    this.role = (data && data.subagentRole) || "";
    this.renderFloating();
    this.applyReadOnly(data && data.status === "CASCADE_RUN_STATUS_RUNNING");
  },

  reset() {
    this.sync(null);
  },

  statusOf(conversationId) {
    const s = this.byId.get(conversationId);
    return s ? s.status || "" : "";
  },

  renderFloating() {
    const card = document.getElementById("subagents-card");
    if (!card) return;
    const running = this.list.filter((s) => s.status === "running");
    if (running.length === 0 || this.parentId) {
      card.classList.add("hidden");
      card.innerHTML = "";
      return;
    }
    card.classList.remove("hidden");
    card.innerHTML =
      `<div class="subagents-header"><div class="ios-spinner subagents-spinner"></div><span>${running.length} 个子代理运行中</span></div>` +
      running
        .map(
          (s) => `<div class="subagents-row" data-id="${escapeHtml(s.conversationId)}">
            <button type="button" class="subagents-open" data-act="open" data-id="${escapeHtml(s.conversationId)}">
              <span class="subagents-name">${escapeHtml(subagentDisplayName(s))}</span>
              <span class="subagents-meta">${escapeHtml(s.typeName || "")}${s.stepCount ? ` · ${s.stepCount} 步` : ""}</span>
            </button>
            <button type="button" class="subagents-stop" data-act="stop" data-id="${escapeHtml(s.conversationId)}" data-feature="subagents" aria-label="关停子代理" title="关停子代理"><i></i></button>
          </div>`,
        )
        .join("");
  },

  // 子代理会话只读：隐藏输入栏，显示提示条（运行中可关停）
  applyReadOnly(isRunning) {
    const view = document.getElementById("view-chat");
    const bar = document.getElementById("subagent-bar");
    const readOnly = !!this.parentId;
    view?.classList.toggle("subagent-readonly", readOnly);
    // 子会话没有列表标题时，导航栏显示它的角色
    const titleEl = document.getElementById("chat-title-text");
    if (readOnly && this.role && titleEl && (!titleEl.textContent || titleEl.textContent === "会话详情")) {
      titleEl.textContent = this.role;
    }
    if (!bar) return;
    bar.classList.toggle("hidden", !readOnly);
    if (!readOnly) {
      bar.innerHTML = "";
      return;
    }
    bar.innerHTML = `<div class="subagent-bar-text"><div class="subagent-bar-title">子代理会话 · 只读</div>${this.role ? `<div class="subagent-bar-role">${escapeHtml(this.role)}</div>` : ""}</div>
      ${isRunning ? '<button type="button" class="subagent-bar-stop" data-act="stop-self" data-feature="subagents">关停</button>' : ""}`;
  },

  async stop(conversationId) {
    if (!conversationId) return;
    if (!confirm("关停这个子代理？\n它正在进行的工作会被中断，已完成的内容会保留。")) return;
    triggerHaptic("medium");
    // 乐观收起：立刻从运行列表里去掉
    const prev = this.list;
    this.list = this.list.map((s) => (s.conversationId === conversationId ? { ...s, status: "done" } : s));
    this.byId = new Map(this.list.map((s) => [s.conversationId, s]));
    this.renderFloating();
    try {
      await postGatewayJson("/gateway/subagent/stop", { conversationId });
    } catch (err) {
      this.list = prev;
      this.byId = new Map(prev.map((s) => [s.conversationId, s]));
      this.renderFloating();
      showToast("关停子代理失败：" + (err.message || "请稍后重试"), 3200);
    }
  },

  // 返回目标：子代理会话回到父会话，其余回到列表
  backHash() {
    return this.parentId ? `#c=${this.parentId}` : "#";
  },
};

function subagentDisplayName(s) {
  const role = (s.role || "").trim();
  if (role) return role;
  const type = (s.typeName || "").trim();
  if (type) return type;
  return String(s.conversationId || "").slice(0, 8);
}

function subagentStatusText(status) {
  return status === "running" ? "运行中" : status === "gone" ? "已清理" : status === "done" ? "已结束" : "";
}

// 消息流里的内联卡片（由 chat.js generateItemHtml 调用）
function buildSubagentCardHtml(sub) {
  const status = SubagentManager.statusOf(sub.conversationId) || "";
  const live = SubagentManager.byId.get(sub.conversationId) || {};
  const name = subagentDisplayName(sub);
  const metaParts = [];
  if (sub.typeName && sub.typeName !== name) metaParts.push(sub.typeName);
  if (status && status !== "done") metaParts.push(subagentStatusText(status));
  if (status === "running" && live.stepCount) metaParts.push(`${live.stepCount} 步`);
  const icon =
    status === "running"
      ? '<div class="ios-spinner subagent-icon-spin"></div>'
      : status === "gone"
      ? '<span class="subagent-icon gone">?</span>'
      : '<span class="subagent-icon done">✓</span>';
  const clickable = status !== "gone";
  return `<button type="button" class="subagent-card" data-id="${escapeHtml(sub.conversationId)}" ${clickable ? "" : "disabled"}>
    ${icon}
    <span class="subagent-main"><span class="subagent-name">${escapeHtml(name)}</span>${metaParts.length ? `<span class="subagent-meta">${escapeHtml(metaParts.join(" · "))}</span>` : ""}</span>
    ${clickable ? '<svg width="8" height="14" viewBox="0 0 8 14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polyline points="1 1 7 7 1 13"></polyline></svg>' : ""}
  </button>`;
}

function initSubagents() {
  document.getElementById("messages-stream")?.addEventListener("click", (e) => {
    const card = e.target.closest(".subagent-card");
    if (!card || card.disabled) return;
    const id = card.getAttribute("data-id");
    if (id) navigateTo(`#c=${id}`);
  });
  document.getElementById("subagents-card")?.addEventListener("click", (e) => {
    const btn = e.target.closest("[data-act]");
    if (!btn) return;
    const id = btn.getAttribute("data-id");
    if (btn.getAttribute("data-act") === "open" && id) navigateTo(`#c=${id}`);
    else if (btn.getAttribute("data-act") === "stop") SubagentManager.stop(id);
  });
  document.getElementById("subagent-bar")?.addEventListener("click", (e) => {
    if (e.target.closest('[data-act="stop-self"]')) SubagentManager.stop(activeCascadeId);
  });
}
