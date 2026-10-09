// Git 提交面板：网关直接调用本机 git（状态 / 提交 / 推送），不再消耗一轮 Agent 对话。
// 接口：POST /gateway/git/status | /gateway/git/commit | /gateway/git/push（都带 cascadeId，网关据此定位仓库）。

const gitState = {
  status: null,
  result: null,
  loading: false,
  working: false,
  error: "",
  actionError: "",
  unchecked: new Set(),
  message: "",
  edited: false,
};

// 默认提交信息：只列文件名，让用户在此基础上改写
function suggestCommitMessage(files) {
  if (!files || !files.length) return "";
  const names = files.map((f) => String(f.path).split("/").pop());
  return names.length <= 3 ? "update " + names.join(", ") : `update ${names.length} files`;
}

function gitStatusLabel(status) {
  switch (status) {
    case "ADDED": return ["A", "add"];
    case "UNTRACKED": return ["U", "add"];
    case "DELETED": return ["D", "del"];
    case "RENAMED": return ["R", "ren"];
    case "CONFLICT": return ["!", "del"];
    default: return ["M", "mod"];
  }
}

function gitNotice(text, tone) {
  return `<div class="sheet-state ${tone || ""}"><p>${escapeHtml(text)}</p></div>`;
}

function renderGitSheet() {
  const body = document.getElementById("git-body");
  if (!body) return;
  const refreshBtn = document.getElementById("btn-git-refresh");
  if (refreshBtn) refreshBtn.disabled = gitState.loading || gitState.working;

  if (gitState.result && gitState.result.committed) {
    const r = gitState.result;
    let line = "已提交";
    if (r.commitId) line += " " + r.commitId;
    if (r.pushed) line += "，已推送";
    const failure = r.pushError || gitState.actionError;
    body.innerHTML = `${gitNotice(line, "ok")}
      ${failure ? `${gitNotice("推送失败：" + failure, "warn")}<button type="button" class="git-btn" data-git="retry-push" ${gitState.working ? "disabled" : ""}>${gitState.working ? "推送中…" : "重试推送"}</button>` : ""}
      <button type="button" class="git-btn primary" data-git="done">完成</button>`;
    return;
  }
  if (gitState.loading && !gitState.status) {
    body.innerHTML = '<div class="sheet-state"><div class="ios-spinner"></div><p>正在读取 Git 状态…</p></div>';
    return;
  }
  if (gitState.error && !gitState.status) {
    body.innerHTML = `${gitNotice(gitState.error, "warn")}<button type="button" class="git-btn" data-git="delegate">改为让 Agent 提交</button>`;
    return;
  }
  const st = gitState.status;
  if (!st) return;

  const head = `<div class="git-branch-row"><span class="git-repo">${escapeHtml(st.repoName || "")}</span>
    <span class="git-branch">${escapeHtml(st.branch || "")}</span>
    ${st.ahead > 0 ? `<span class="git-ahead">↑${st.ahead}</span>` : ""}${st.behind > 0 ? `<span class="git-behind">↓${st.behind}</span>` : ""}
    ${!st.upstream && !st.detached ? '<span class="git-noup">未设置上游</span>' : ""}</div>`;

  if (st.clean) {
    const canPush = st.ahead > 0 && st.upstream;
    body.innerHTML = `${head}${gitNotice("工作区没有可提交的改动", "ok")}
      ${canPush ? `<button type="button" class="git-btn primary" data-git="retry-push" ${gitState.working ? "disabled" : ""}>${gitState.working ? "推送中…" : `推送 ${st.ahead} 个未推送的提交`}</button>` : ""}
      ${gitState.actionError ? `<div class="git-error">${escapeHtml(gitState.actionError)}</div>` : ""}`;
    return;
  }

  const selected = st.files.filter((f) => !gitState.unchecked.has(f.path));
  if (!gitState.edited) gitState.message = suggestCommitMessage(selected);
  const canCommit = !gitState.working && selected.length > 0 && gitState.message.trim().length > 0;

  const rows = st.files
    .map((f) => {
      const on = !gitState.unchecked.has(f.path);
      const [label, tone] = gitStatusLabel(f.status);
      const parts = String(f.path).split("/");
      const name = parts.pop();
      const dir = parts.join("/");
      return `<button type="button" class="git-file" data-path="${escapeHtml(f.path)}">
        <span class="git-check${on ? " on" : ""}" aria-hidden="true"></span>
        <span class="git-flag ${tone}">${label}</span>
        <span class="git-file-main"><span class="git-file-name">${escapeHtml(name)}</span>${dir ? `<span class="git-file-dir">${escapeHtml(dir)}</span>` : ""}</span>
      </button>`;
    })
    .join("");

  body.innerHTML = `${head}
    <div class="git-section-title">变更文件</div><div class="git-files">${rows}</div>
    <div class="git-section-title">提交信息</div>
    <textarea id="git-message" class="git-message" rows="3" placeholder="提交信息">${escapeHtml(gitState.message)}</textarea>
    ${gitState.actionError ? `<div class="git-error">${escapeHtml(gitState.actionError)}</div>` : ""}
    <div class="git-actions">
      <button type="button" class="git-btn" data-git="commit" ${canCommit ? "" : "disabled"}>提交</button>
      <button type="button" class="git-btn primary" data-git="commit-push" ${canCommit ? "" : "disabled"}>${gitState.working ? "处理中…" : "提交并推送"}</button>
    </div>
    <button type="button" class="git-link" data-git="delegate" ${gitState.working ? "disabled" : ""}>让 Agent 写提交信息并提交</button>`;
}

async function refreshGitStatus() {
  if (!activeCascadeId) return;
  const id = activeCascadeId;
  gitState.loading = true;
  gitState.error = "";
  renderGitSheet();
  try {
    const st = await postGatewayJson("/gateway/git/status", { cascadeId: id });
    if (id !== activeCascadeId) return;
    gitState.status = st;
    // 刷新后丢弃已不存在的文件的取消勾选状态
    const paths = new Set((st.files || []).map((f) => f.path));
    gitState.unchecked = new Set([...gitState.unchecked].filter((p) => paths.has(p)));
  } catch (err) {
    gitState.error = err.message || "读取 Git 状态失败";
  }
  gitState.loading = false;
  renderGitSheet();
}

function openGitSheet() {
  if (!activeCascadeId) {
    showToast("新会话还没有关联的工作区");
    return;
  }
  const sheet = document.getElementById("sheet-git");
  if (!sheet) return;
  Object.assign(gitState, { status: null, result: null, error: "", actionError: "", unchecked: new Set(), message: "", edited: false, working: false });
  renderGitSheet();
  sheet.classList.remove("hidden");
  refreshGitStatus();
}

function closeGitSheet() {
  document.getElementById("sheet-git")?.classList.add("hidden");
}

// 保留原有行为：把「Commit and Push」放进输入框，让 Agent 来写提交信息并提交
function delegateCommitToAgent() {
  closeGitSheet();
  const input = document.getElementById("chat-input");
  if (!input) return;
  const text = "Commit and Push";
  input.value = input.value.trim() ? input.value + "\n" + text : text;
  input.dispatchEvent(new Event("input", { bubbles: true }));
  input.focus();
  if (activeCascadeId) DraftManager.set(activeCascadeId, input.value);
}

async function gitCommit(push) {
  const st = gitState.status;
  if (!st || gitState.working) return;
  const selected = st.files.map((f) => f.path).filter((p) => !gitState.unchecked.has(p));
  // 全选时传空数组 = 提交全部（含之后新出现的），部分选择才传明确路径
  const paths = selected.length === st.files.length ? [] : selected;
  gitState.working = true;
  gitState.actionError = "";
  renderGitSheet();
  try {
    gitState.result = await postGatewayJson("/gateway/git/commit", {
      cascadeId: activeCascadeId,
      message: gitState.message,
      paths,
      push,
    });
  } catch (err) {
    gitState.actionError = err.message || "提交失败";
  }
  gitState.working = false;
  renderGitSheet();
}

// 提交已成功但推送失败时单独重试；工作区干净但有未推送提交时也走这里
async function retryGitPush() {
  if (gitState.working) return;
  gitState.working = true;
  gitState.actionError = "";
  renderGitSheet();
  try {
    await postGatewayJson("/gateway/git/push", { cascadeId: activeCascadeId });
    if (gitState.result) {
      gitState.result = { ...gitState.result, pushed: true, pushError: "" };
    } else {
      gitState.result = null;
    }
    gitState.working = false;
    if (!gitState.result) {
      await refreshGitStatus(); // 干净工作区直接推送：刷新状态，ahead 会归零
      return;
    }
  } catch (err) {
    gitState.actionError = err.message || "推送失败";
    gitState.working = false;
  }
  renderGitSheet();
}

function initGitSheet() {
  document.getElementById("btn-commit-push")?.addEventListener("click", openGitSheet);
  document.getElementById("btn-git-close")?.addEventListener("click", closeGitSheet);
  document.getElementById("btn-git-refresh")?.addEventListener("click", refreshGitStatus);
  const sheet = document.getElementById("sheet-git");
  sheet?.addEventListener("click", (e) => {
    if (e.target === sheet) closeGitSheet();
  });
  const body = document.getElementById("git-body");
  body?.addEventListener("click", (e) => {
    const fileBtn = e.target.closest(".git-file");
    if (fileBtn) {
      const path = fileBtn.getAttribute("data-path");
      if (gitState.unchecked.has(path)) gitState.unchecked.delete(path);
      else gitState.unchecked.add(path);
      renderGitSheet();
      return;
    }
    const action = e.target.closest("[data-git]")?.getAttribute("data-git");
    if (action === "commit") gitCommit(false);
    else if (action === "commit-push") gitCommit(true);
    else if (action === "retry-push") retryGitPush();
    else if (action === "delegate") delegateCommitToAgent();
    else if (action === "done") closeGitSheet();
  });
  body?.addEventListener("input", (e) => {
    if (e.target.id !== "git-message") return;
    gitState.message = e.target.value;
    gitState.edited = true;
    // 只刷新按钮可用状态，不重绘（否则输入框会失焦）
    const hasSelected = !!gitState.status && gitState.status.files.some((f) => !gitState.unchecked.has(f.path));
    const ok = !gitState.working && hasSelected && gitState.message.trim().length > 0;
    body.querySelectorAll('[data-git="commit"],[data-git="commit-push"]').forEach((b) => {
      b.disabled = !ok;
    });
  });
}
