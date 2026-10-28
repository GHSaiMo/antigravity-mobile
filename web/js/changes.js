// 「本会话改动」：Agent 在当前会话里累计改动了哪些文件，点开可看正向 diff。
// 数据来自网关 POST /gateway/cascade/changes（基于 GetRevertPreview 反转得到）。

const changesState = { data: null, viewing: null, loading: false, error: "" };
const DIFF_MAX_LINES = 4000;

function changesActionBadge(action) {
  const a = String(action || "").toUpperCase();
  if (a === "CREATE") return '<span class="diff-badge create">新增</span>';
  if (a === "DELETE") return '<span class="diff-badge delete">删除</span>';
  return '<span class="diff-badge modify">修改</span>';
}

function tokenizeDiffText(text) {
  if (!text) return [" "];
  const tokens = [];
  const re = /[a-zA-Z0-9_]+|\s+|[^\s\w]/g;
  let m;
  while ((m = re.exec(text)) !== null) {
    tokens.push(m[0]);
  }
  return tokens.length ? tokens : [text];
}

function computeTokenDiff(oldText, newText) {
  const tokensA = tokenizeDiffText(oldText || " ");
  const tokensB = tokenizeDiffText(newText || " ");
  const m = tokensA.length;
  const n = tokensB.length;
  const dp = Array.from({ length: m + 1 }, () => new Uint16Array(n + 1));
  for (let i = 1; i <= m; i++) {
    for (let j = 1; j <= n; j++) {
      if (tokensA[i - 1] === tokensB[j - 1]) {
        dp[i][j] = dp[i - 1][j - 1] + 1;
      } else {
        dp[i][j] = Math.max(dp[i - 1][j], dp[i][j - 1]);
      }
    }
  }
  const commonA = new Set();
  const commonB = new Set();
  let i = m, j = n;
  while (i > 0 && j > 0) {
    if (tokensA[i - 1] === tokensB[j - 1]) {
      commonA.add(i - 1);
      commonB.add(j - 1);
      i--; j--;
    } else if (dp[i - 1][j] >= dp[i][j - 1]) {
      i--;
    } else {
      j--;
    }
  }
  const spansA = tokensA.map((t, idx) => ({ text: t, isChanged: !commonA.has(idx) }));
  const spansB = tokensB.map((t, idx) => ({ text: t, isChanged: !commonB.has(idx) }));
  return [spansA, spansB];
}

function processDiffLines(lines) {
  const result = [];
  let i = 0;
  const n = lines.length;
  while (i < n) {
    const type = String(lines[i].type || "").toUpperCase();
    if (type === "DELETE") {
      let delEnd = i;
      while (delEnd < n && String(lines[delEnd].type || "").toUpperCase() === "DELETE") delEnd++;
      let insEnd = delEnd;
      while (insEnd < n && String(lines[insEnd].type || "").toUpperCase() === "INSERT") insEnd++;
      const delCount = delEnd - i;
      const insCount = insEnd - delEnd;
      if (insCount > 0) {
        const pairCount = Math.min(delCount, insCount);
        for (let k = 0; k < pairCount; k++) {
          const [delSpans] = computeTokenDiff(lines[i + k].text, lines[delEnd + k].text);
          result.push({ line: lines[i + k], spans: delSpans });
        }
        for (let k = pairCount; k < delCount; k++) {
          result.push({ line: lines[i + k], spans: [{ text: lines[i + k].text || " ", isChanged: false }] });
        }
        for (let k = 0; k < pairCount; k++) {
          const [, insSpans] = computeTokenDiff(lines[i + k].text, lines[delEnd + k].text);
          result.push({ line: lines[delEnd + k], spans: insSpans });
        }
        for (let k = pairCount; k < insCount; k++) {
          result.push({ line: lines[delEnd + k], spans: [{ text: lines[delEnd + k].text || " ", isChanged: false }] });
        }
        i = insEnd;
        continue;
      }
    }
    result.push({ line: lines[i], spans: [{ text: lines[i].text || " ", isChanged: false }] });
    i++;
  }
  return result;
}

function renderChangesSheet() {
  const body = document.getElementById("changes-body");
  const title = document.getElementById("changes-title");
  const backBtn = document.getElementById("btn-changes-back");
  const closeBtn = document.getElementById("btn-changes-close");
  if (!body) return;

  // 单文件 diff
  if (changesState.viewing) {
    const f = changesState.viewing;
    title.textContent = f.fileName || "改动详情";
    backBtn.classList.remove("hidden");
    closeBtn.classList.add("hidden");
    const lines = Array.isArray(f.diffLines) ? f.diffLines : [];
    const shown = lines.slice(0, DIFF_MAX_LINES);
    const processed = processDiffLines(shown);
    const rows = processed
      .map(({ line: l, spans }) => {
        const t = String(l.type || "").toUpperCase();
        const cls = t === "INSERT" ? "ins" : t === "DELETE" ? "del" : "ctx";
        const sign = t === "INSERT" ? "+" : t === "DELETE" ? "−" : " ";
        const spanHtml = spans.map(s => {
          const escaped = escapeHtml(s.text);
          if (!s.isChanged) return escaped;
          const tokenCls = t === "INSERT" ? "diff-token-ins" : "diff-token-del";
          return `<span class="${tokenCls}">${escaped}</span>`;
        }).join("");
        return `<div class="diff-line ${cls}"><span class="diff-sign">${sign}</span><span class="diff-text">${spanHtml || "&nbsp;"}</span></div>`;
      })
      .join("");
    const truncated = lines.length > shown.length ? `<div class="diff-truncated">仅显示前 ${DIFF_MAX_LINES} 行，共 ${lines.length} 行</div>` : "";
    body.innerHTML = `
      <div class="diff-file-head">${changesActionBadge(f.actionType)}
        <span class="diff-count add">+${f.additions || 0}</span><span class="diff-count del">−${f.deletions || 0}</span>
        <span class="diff-path" title="${escapeHtml(f.fileUri || "")}">${escapeHtml((f.fileUri || "").replace(/^file:\/\//, ""))}</span></div>
      <div class="diff-view">${rows || '<div class="diff-empty">没有可显示的差异</div>'}</div>${truncated}`;
    return;
  }

  title.textContent = "本会话改动";
  backBtn.classList.add("hidden");
  closeBtn.classList.remove("hidden");

  if (changesState.loading) {
    body.innerHTML = '<div class="sheet-state"><div class="ios-spinner"></div><p>正在汇总代码改动…</p></div>';
    return;
  }
  if (changesState.error) {
    body.innerHTML = `<div class="sheet-state warn"><p>${escapeHtml(changesState.error)}</p></div>`;
    return;
  }
  const data = changesState.data;
  if (!data || !data.hasChanges || !(data.files || []).length) {
    body.innerHTML = '<div class="sheet-state ok"><p>本会话没有产生文件改动</p></div>';
    return;
  }
  body.innerHTML =
    `<div class="changes-summary"><span>${data.files.length} 个文件</span><span class="diff-count add">+${data.additions || 0}</span><span class="diff-count del">−${data.deletions || 0}</span></div>` +
    data.files
      .map(
        (f, i) => `<button type="button" class="changes-file" data-index="${i}">
          <div class="changes-file-main">
            <div class="changes-file-name">${escapeHtml(f.fileName || "")}</div>
            <div class="changes-file-meta">${changesActionBadge(f.actionType)}
              ${f.additions > 0 ? `<span class="diff-count add">+${f.additions}</span>` : ""}${f.deletions > 0 ? `<span class="diff-count del">−${f.deletions}</span>` : ""}</div>
          </div>
          <svg width="8" height="14" viewBox="0 0 8 14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polyline points="1 1 7 7 1 13"></polyline></svg>
        </button>`,
      )
      .join("");
}

async function openChangesSheet() {
  if (!activeCascadeId) {
    showToast("新会话还没有任何改动");
    return;
  }
  const sheet = document.getElementById("sheet-changes");
  if (!sheet) return;
  changesState.viewing = null;
  changesState.error = "";
  changesState.data = null;
  changesState.loading = true;
  renderChangesSheet();
  sheet.classList.remove("hidden");
  const id = activeCascadeId;
  try {
    const data = await postGatewayJson("/gateway/cascade/changes", { cascadeId: id });
    if (id !== activeCascadeId) return;
    changesState.data = data;
  } catch (err) {
    changesState.error = err.message || "读取改动失败";
  }
  changesState.loading = false;
  renderChangesSheet();
}

function closeChangesSheet() {
  document.getElementById("sheet-changes")?.classList.add("hidden");
  changesState.viewing = null;
}

function initChangesSheet() {
  document.getElementById("btn-changes")?.addEventListener("click", openChangesSheet);
  document.getElementById("btn-changes-close")?.addEventListener("click", closeChangesSheet);
  document.getElementById("btn-changes-back")?.addEventListener("click", () => {
    changesState.viewing = null;
    renderChangesSheet();
  });
  const sheet = document.getElementById("sheet-changes");
  sheet?.addEventListener("click", (e) => {
    if (e.target === sheet) closeChangesSheet();
  });
  document.getElementById("changes-body")?.addEventListener("click", (e) => {
    const card = e.target.closest(".changes-file");
    if (!card || !changesState.data) return;
    const f = (changesState.data.files || [])[Number(card.getAttribute("data-index"))];
    if (!f) return;
    changesState.viewing = f;
    renderChangesSheet();
    document.getElementById("changes-body").scrollTop = 0;
  });
}
