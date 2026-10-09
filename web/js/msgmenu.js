// 消息操作条（点一下气泡，在它下方展开）、轻提示、导出 Markdown。
//
// 为什么不用长按菜单：触摸设备上长按文字本来就是系统的「选字」手势，和自定义菜单会冲突；
// 点一下展开一条操作条不占位、也不抢手势。

// ---------------------------------------------------------------------------
// 轻提示
// ---------------------------------------------------------------------------
let toastTimer = 0;

function showToast(message, ms = 1800) {
  let el = document.getElementById("app-toast");
  if (!el) {
    el = document.createElement("div");
    el.id = "app-toast";
    el.className = "app-toast";
    el.setAttribute("role", "status");
    document.body.appendChild(el);
  }
  el.textContent = message;
  el.classList.add("show");
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => el.classList.remove("show"), ms);
}

// ---------------------------------------------------------------------------
// 复制
// ---------------------------------------------------------------------------
async function copyTextToClipboard(text) {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch (_) {}
  // 非安全上下文（局域网 http）没有 navigator.clipboard，退回 execCommand
  const ta = document.createElement("textarea");
  ta.value = text;
  ta.setAttribute("readonly", "");
  ta.style.cssText = "position:fixed;top:0;left:0;opacity:0;";
  document.body.appendChild(ta);
  ta.select();
  let ok = false;
  try {
    ok = document.execCommand("copy");
  } catch (_) {}
  ta.remove();
  return ok;
}

// ---------------------------------------------------------------------------
// 导出 Markdown（language_server ConvertTrajectoryToMarkdown，经网关透传）
// ---------------------------------------------------------------------------
function sanitizeFileName(name) {
  const cleaned = String(name || "")
    .replace(/[\\/:*?"<>|\u0000-\u001f]/g, " ")
    .replace(/\s+/g, " ")
    .trim()
    .slice(0, 60);
  return cleaned || "会话";
}

function downloadTextFile(fileName, text, mime = "text/markdown") {
  const blob = new Blob([text], { type: `${mime};charset=utf-8` });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = fileName;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 4000);
}

let exportingMarkdown = false;

async function exportConversationMarkdown() {
  if (exportingMarkdown) return;
  const cascadeId = typeof activeCascadeId !== "undefined" ? activeCascadeId : "";
  if (!cascadeId) return;
  exportingMarkdown = true;
  showToast("正在导出…", 8000);
  try {
    const data = await rpc("ConvertTrajectoryToMarkdown", { conversationId: cascadeId });
    const markdown = data && typeof data.markdown === "string" ? data.markdown : "";
    if (!markdown.trim()) {
      showToast("会话内容为空，没有可导出的内容");
      return;
    }
    // 直接用链接打开会话时导航栏标题可能还是占位文字，优先用会话列表里的真实标题
    let info = typeof currentTrajectories !== "undefined" ? currentTrajectories?.[cascadeId] : null;
    if (!info) {
      // 直接用链接进入会话时列表还没加载过，补取一次拿标题
      try {
        const all = await rpc("GetAllCascadeTrajectories");
        info = all?.trajectorySummaries?.[cascadeId] || null;
      } catch (_) {}
    }
    const listTitle = info && typeof formatConversationTitle === "function" ? formatConversationTitle(info.annotations, info.summary, "") : "";
    const navTitle = document.getElementById("chat-title-text")?.textContent || "";
    const title = listTitle || (navTitle && navTitle !== "会话详情" ? navTitle : "") || "会话";
    const fileName = `${sanitizeFileName(title)}.md`;
    const file = typeof File === "function" ? new File([markdown], fileName, { type: "text/markdown" }) : null;
    // 手机上优先走系统分享面板；不支持文件分享的浏览器（如桌面 Chrome / Firefox）直接下载
    if (file && navigator.canShare && navigator.canShare({ files: [file] })) {
      try {
        await navigator.share({ files: [file], title });
        showToast("已导出");
        return;
      } catch (err) {
        if (err && err.name === "AbortError") {
          showToast("已取消");
          return;
        }
      }
    }
    downloadTextFile(fileName, markdown);
    showToast("已导出为 " + fileName);
  } catch (err) {
    showToast("导出失败：" + (err && err.message ? err.message : "未知错误"), 3200);
  } finally {
    exportingMarkdown = false;
  }
}

// ---------------------------------------------------------------------------
// 气泡操作条
// ---------------------------------------------------------------------------
function closeMessageActions(exceptRow) {
  document.querySelectorAll("#messages-stream .message-row.actions-open").forEach((row) => {
    if (row !== exceptRow) row.classList.remove("actions-open");
  });
}

// 点击这些元素时不展开操作条（它们有自己的行为）
const MESSAGE_ACTION_IGNORE = "a, button, summary, details, input, textarea, select, img, video, .code-copy-btn, .artifact-preview-card, .thumbnail-card, [data-action]";

function messageRowText(row) {
  const body = row.querySelector(".agent-message-body") || row.querySelector(".bubble");
  return body ? body.innerText.trim() : "";
}

function initMessageActions() {
  const stream = document.getElementById("messages-stream");
  if (!stream) return;

  stream.addEventListener("click", async (e) => {
    const actionBtn = e.target.closest(".msg-action-btn");
    if (actionBtn) {
      const row = actionBtn.closest(".message-row");
      const action = actionBtn.getAttribute("data-msg-action");
      if (typeof triggerHaptic === "function") triggerHaptic("light");
      if (action === "copy" && row) {
        const ok = await copyTextToClipboard(messageRowText(row));
        showToast(ok ? "已复制" : "复制失败，请长按文字手动选择");
      } else if (action === "export") {
        exportConversationMarkdown();
      }
      row?.classList.remove("actions-open");
      return;
    }

    const bubble = e.target.closest(".bubble");
    if (!bubble) {
      closeMessageActions();
      return;
    }
    if (e.target.closest(MESSAGE_ACTION_IGNORE)) return;
    // 正在选字时不展开，避免打断选择
    const sel = window.getSelection && window.getSelection();
    if (sel && !sel.isCollapsed && sel.toString().length > 0) return;

    const row = bubble.closest(".message-row");
    if (!row || !row.querySelector(".msg-actions")) return;
    const willOpen = !row.classList.contains("actions-open");
    closeMessageActions(row);
    row.classList.toggle("actions-open", willOpen);
  });
}

// 生成操作条 HTML。kind: "user" | "agent"
function buildMessageActionsHtml(kind) {
  const icon = {
    copy: '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="9" y="9" width="13" height="13" rx="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg>',
    export: '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path><polyline points="7 10 12 15 17 10"></polyline><line x1="12" y1="15" x2="12" y2="3"></line></svg>',
  };
  let html = `<button type="button" class="msg-action-btn" data-msg-action="copy">${icon.copy}<span>复制</span></button>`;
  if (kind === "agent") {
    html += `<button type="button" class="msg-action-btn" data-msg-action="export" data-feature="export">${icon.export}<span>导出 MD</span></button>`;
  }
  return `<div class="msg-actions" data-msg-kind="${kind}">${html}</div>`;
}
