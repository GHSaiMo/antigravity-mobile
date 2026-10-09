// 斜杠命令：输入框里键入 "/" 弹出命令列表（系统命令 + 技能，与桌面端 "/" 菜单同源）；
// 选中后命令变成输入框上方的标签，发送时作为 items 里的 slashCommand 条目随消息一起发出。
// 网关用权威定义替换条目内容，客户端只传名字（见 internal/proxy/slash_commands.go）。

const slashState = {
  commands: [],
  loading: false,
  loaded: false,
  selected: null, // { name, title, kind }
  cascadeId: "", // 选中该命令时所在的会话
};

// 输入以 "/" 开头且尚未出现空白时，返回要筛选的关键字（不含 "/"）；否则返回 null，表示不弹出列表。
function slashQueryOf(input) {
  if (typeof input !== "string" || !input.startsWith("/")) return null;
  const rest = input.slice(1);
  return /\s/.test(rest) ? null : rest;
}

// 名称前缀命中优先，其次名称 / 标题 / 描述包含；保持原有顺序。
function filterSlashCommands(all, query) {
  const q = String(query || "").trim().toLowerCase();
  if (!q) return all;
  const prefix = all.filter((c) => c.name.toLowerCase().startsWith(q));
  const rest = all.filter(
    (c) =>
      !prefix.includes(c) &&
      (c.name.toLowerCase().includes(q) || (c.title || "").toLowerCase().includes(q) || (c.description || "").toLowerCase().includes(q)),
  );
  return prefix.concat(rest);
}

function getSelectedSlashCommand() {
  // 标签只属于选中它的那个会话：切换会话后自动失效
  if (slashState.selected && slashState.cascadeId !== activeCascadeId) {
    slashState.selected = null;
    renderSlashChip();
  }
  return slashState.selected;
}

// 发出去的 items：命令条目在前，用户文字（前面补一个空格）在后
function buildSendItems(text) {
  const items = [];
  if (slashState.selected) {
    items.push({ item: { slashCommand: { info: { name: slashState.selected.name } } } });
    if (text) items.push({ text: " " + text });
    return items;
  }
  if (text) items.push({ text });
  return items;
}

// 乐观气泡 / 队列里显示的文字
function slashDisplayText(text) {
  if (!slashState.selected) return text;
  return "/" + slashState.selected.name + (text ? " " + text : "");
}

function clearSelectedSlash() {
  slashState.selected = null;
  renderSlashChip();
}

// 首次键入 "/" 时懒加载；已加载或正在加载时什么都不做，失败后下次键入 "/" 会重试。
async function ensureSlashCommands() {
  if (slashState.loaded || slashState.loading) return;
  slashState.loading = true;
  renderSlashPicker();
  try {
    const resp = await fetch("/gateway/slash-commands");
    if (resp.ok) {
      const data = await resp.json();
      slashState.commands = Array.isArray(data.commands) ? data.commands : [];
      slashState.loaded = true;
    }
  } catch (_) {
    // 保持未加载，下次键入 "/" 重试
  } finally {
    slashState.loading = false;
    renderSlashPicker();
  }
}

function slashPickerQuery() {
  if (!activeCascadeId) return null; // 新建会话（草稿）不支持斜杠命令
  if (!isFeatureAvailable(GATEWAY_FEATURE.SLASH)) return null;
  const input = document.getElementById("chat-input");
  return slashQueryOf(input ? input.value : "");
}

function renderSlashPicker() {
  const box = document.getElementById("slash-picker");
  if (!box) return;
  const query = slashPickerQuery();
  if (query === null) {
    box.classList.add("hidden");
    box.innerHTML = "";
    return;
  }
  const list = filterSlashCommands(slashState.commands, query);
  box.classList.remove("hidden");
  if (list.length === 0) {
    box.innerHTML = `<div class="slash-empty">${
      slashState.loading ? '<div class="ios-spinner slash-spinner"></div><span>正在读取命令…</span>' : "<span>没有匹配的命令</span>"
    }</div>`;
    return;
  }
  box.innerHTML = list
    .map((c) => {
      const kind = c.kind === "skill" ? "技能" : "系统";
      const desc = c.description ? `<span class="slash-item-desc">${escapeHtml(c.description)}</span>` : "";
      return `<button type="button" class="slash-item" data-name="${escapeHtml(c.name)}">
        <span class="slash-item-main"><span class="slash-item-name">/${escapeHtml(c.name)}</span><span class="slash-item-kind">${kind}</span></span>${desc}
      </button>`;
    })
    .join("");
}

function renderSlashChip() {
  const chip = document.getElementById("slash-chip");
  if (!chip) return;
  if (!slashState.selected) {
    chip.classList.add("hidden");
    chip.innerHTML = "";
  } else {
    chip.classList.remove("hidden");
    chip.innerHTML = `<span class="slash-chip-name">/${escapeHtml(slashState.selected.name)}</span><button type="button" class="slash-chip-clear" aria-label="移除命令">×</button>`;
  }
  const isRunning = typeof currentTrajectories !== "undefined" && currentTrajectories[activeCascadeId]?.status === "CASCADE_RUN_STATUS_RUNNING";
  if (typeof updateChatControls === "function") updateChatControls(isRunning, null, false);
}

function selectSlashCommand(name) {
  const cmd = slashState.commands.find((c) => c.name === name);
  if (!cmd) return;
  if (typeof triggerHaptic === "function") triggerHaptic("light");
  const input = document.getElementById("chat-input");
  if (input) {
    input.value = "";
    input.style.height = "auto";
    input.focus();
  }
  slashState.selected = { name: cmd.name, title: cmd.title || cmd.name, kind: cmd.kind };
  slashState.cascadeId = activeCascadeId;
  renderSlashPicker();
  renderSlashChip();
}

function initSlashCommands() {
  const input = document.getElementById("chat-input");
  input?.addEventListener("input", () => {
    if (slashPickerQuery() !== null) ensureSlashCommands();
    renderSlashPicker();
  });
  document.getElementById("slash-picker")?.addEventListener("click", (e) => {
    const item = e.target.closest(".slash-item");
    if (item) selectSlashCommand(item.getAttribute("data-name"));
  });
  document.getElementById("slash-chip")?.addEventListener("click", (e) => {
    if (e.target.closest(".slash-chip-clear")) clearSelectedSlash();
  });
}
