// ==========================================================================
// Cockpit Quota Monitor Module
// ==========================================================================
// 功能参数：是否在 Cockpit Tools 页面显示账号切换按钮。
// 网关会先退出电脑上的 Antigravity，再让 Cockpit 注入 token 并重启 IDE。
const ENABLE_COCKPIT_SWITCH = true;

function isCockpitSwitchEnabled() {
  if (typeof window !== "undefined" && typeof window.ENABLE_COCKPIT_SWITCH === "boolean") {
    return window.ENABLE_COCKPIT_SWITCH;
  }
  return ENABLE_COCKPIT_SWITCH;
}

let currentCockpitQuotas = null;

async function fetchCockpitQuotas(isManual = false) {
  const refreshBtn = document.getElementById("btn-quota-refresh");
  const refreshIcon = refreshBtn ? refreshBtn.querySelector(".refresh-icon") : null;
  if (isManual && refreshIcon) {
    refreshIcon.classList.add("spinning");
  }

  try {
    if (isManual) {
      const initialUpdatedAt = currentCockpitQuotas?.updated_at || 0;
      const refreshResp = await fetch("/api/v1/cockpit/refresh", { method: "POST" }).catch(() => null);
      if (refreshResp && refreshResp.ok) {
        const directData = await refreshResp.json().catch(() => null);
        if (directData && directData.updated_at > initialUpdatedAt) {
          currentCockpitQuotas = directData;
          renderQuotaStatusBar(directData);
          renderQuotaSheet(directData);
          return;
        }
      }
      // Poll for up to 20s if background batch refresh across accounts takes longer
      const startTime = Date.now();
      while (Date.now() - startTime < 20000) {
        await new Promise((r) => setTimeout(r, 1500));
        const pResp = await fetch("/api/v1/cockpit/quotas").catch(() => null);
        if (pResp && pResp.ok) {
          const pData = await pResp.json().catch(() => null);
          if (pData && pData.updated_at > initialUpdatedAt) {
            currentCockpitQuotas = pData;
            renderQuotaStatusBar(pData);
            renderQuotaSheet(pData);
            return;
          }
        }
      }
    }
    const resp = await fetch("/api/v1/cockpit/quotas");
    if (!resp.ok) throw new Error("HTTP " + resp.status);
    const data = await resp.json();
    currentCockpitQuotas = data;
    renderQuotaStatusBar(data);
    renderQuotaSheet(data);
  } catch (err) {
    console.warn("[CockpitQuota] Failed to fetch quotas:", err);
    const statusDesc = document.getElementById("quota-status-desc");
    if (statusDesc && !currentCockpitQuotas) {
      statusDesc.textContent = "未能连接 Cockpit";
    }
  } finally {
    if (refreshIcon) {
      refreshIcon.classList.remove("spinning");
    }
  }
}

function getQuotaStatusClass(percent) {
  if (percent >= 50) return "good";
  if (percent >= 20) return "warning";
  return "danger";
}

function formatResetClockTime(isoStr) {
  if (!isoStr) return "";
  try {
    const d = new Date(isoStr);
    if (isNaN(d.getTime())) return "";
    const hours = String(d.getHours()).padStart(2, '0');
    const minutes = String(d.getMinutes()).padStart(2, '0');
    return `(${hours}:${minutes})`;
  } catch (e) {
    return "";
  }
}

function formatResetDateTime(isoStr) {
  if (!isoStr) return "";
  try {
    const d = new Date(isoStr);
    if (isNaN(d.getTime())) return "";
    const month = String(d.getMonth() + 1).padStart(2, '0');
    const day = String(d.getDate()).padStart(2, '0');
    const hours = String(d.getHours()).padStart(2, '0');
    const minutes = String(d.getMinutes()).padStart(2, '0');
    return `(${month}/${day} ${hours}:${minutes})`;
  } catch (e) {
    return "";
  }
}

function getQuotaResetDisplayText(bucket) {
  if (!bucket || !bucket.reset_friendly) return "配额充足";
  const friendly = bucket.reset_friendly;
  if (friendly === "已就绪" || friendly === "未知" || friendly === "配额充足" || friendly === "就绪") {
    return friendly;
  }
  const dateTime = formatResetDateTime(bucket.reset_time);
  if (dateTime) {
    return `${friendly} ${dateTime}`;
  }
  return friendly;
}

function renderQuotaStatusBar(data) {
  if (!data) return;
  const current = data.current_account || (data.accounts && data.accounts[0]);
  const statusBar = document.getElementById("quota-status-bar");
  if (!current) {
    if (statusBar) statusBar.style.display = "none";
    return;
  }

  const isFree = (current.subscription_tier && current.subscription_tier.toUpperCase() === "FREE") ||
    (!current.gemini_5h && !current.claude_5h);
  const metric = isFree ? (current.gemini_weekly || current.claude_weekly) : (current.gemini_5h || current.gemini_weekly);
  if (!metric) {
    if (statusBar) statusBar.style.display = "none";
    return;
  }
  if (statusBar) statusBar.style.display = "";

  const titleText = isFree
    ? (current.gemini_weekly ? "Gemini Weekly" : "Claude Weekly")
    : (current.gemini_5h ? "Gemini 5h" : "Gemini Weekly");

  const nameEl = statusBar.querySelector(".quota-metric-name");
  if (nameEl) nameEl.textContent = titleText;

  const percentEl = document.getElementById("quota-status-percent");
  const fillEl = document.getElementById("quota-status-fill");
  const descEl = document.getElementById("quota-status-desc");

  const pct = metric.remaining_percent;
  const statusClass = getQuotaStatusClass(pct);

  const boltEl = document.getElementById("quota-bolt-icon");
  if (boltEl) boltEl.setAttribute("class", `quota-bolt-icon ${statusClass === "good" ? "" : statusClass}`);
  if (percentEl) percentEl.textContent = `${pct.toFixed(1)}%`;
  if (fillEl) {
    fillEl.style.width = `${Math.min(100, Math.max(0, pct))}%`;
    fillEl.className = `quota-progress-fill fill-${statusClass}`;
  }
  if (descEl) {
    const resetTxt = metric.reset_friendly || "就绪";
    let displayText = resetTxt;
    if (resetTxt !== "就绪" && resetTxt !== "已就绪" && resetTxt !== "未知") {
      const dateTime = formatResetDateTime(metric.reset_time);
      if (dateTime) {
        displayText = `${resetTxt} ${dateTime}`;
      }
    }
    descEl.textContent = displayText;
  }
}

let isCockpitEmailMasked = localStorage.getItem("cockpit_email_masked") === "true";

function maskEmail(email) {
  if (!email || typeof email !== "string") return email;
  const atIdx = email.indexOf("@");
  if (atIdx <= 0) return email;
  const local = email.slice(0, atIdx);
  const domain = email.slice(atIdx + 1);

  const maskPart = (str) => {
    if (str.length <= 1) return str + "*";
    if (str.length === 2) return str[0] + "*" + str[1];
    return str[0] + "*".repeat(str.length - 2) + str[str.length - 1];
  };

  const dotIdx = domain.lastIndexOf(".");
  if (dotIdx > 0) {
    const domainName = domain.slice(0, dotIdx);
    const domainExt = domain.slice(dotIdx);
    return `${maskPart(local)}@${maskPart(domainName)}${domainExt}`;
  }
  return `${maskPart(local)}@${maskPart(domain)}`;
}

function getTierBadgeHtml(tier) {
  if (!tier || typeof tier !== "string") return "";
  const norm = tier.trim().toUpperCase();
  if (norm === "ULTRA") {
    return `<span class="quota-tier-badge ultra">ULTRA</span>`;
  }
  if (norm === "PRO") {
    return `<span class="quota-tier-badge pro">PRO</span>`;
  }
  if (norm === "FREE") {
    return `<span class="quota-tier-badge free">FREE</span>`;
  }
  return "";
}

function calculateQuotaPace(title, bucket, tier) {
  if (!bucket || !bucket.reset_time) return null;
  const resetDate = new Date(bucket.reset_time);
  const remainingMillis = resetDate.getTime() - Date.now();
  if (isNaN(remainingMillis) || remainingMillis <= 0) return null;
  const lowerTitle = (title || "").toLowerCase();
  let is5h = false;
  if (lowerTitle.includes("argon")) {
    const isUltra = (tier && typeof tier === "string") && tier.toUpperCase().includes("ULTRA");
    is5h = isUltra;
  } else if (lowerTitle.includes("5h")) {
    is5h = true;
  } else {
    is5h = false;
  }
  const totalMillis = is5h ? 5 * 3600 * 1000 : 7 * 24 * 3600 * 1000;
  const windowLabel = is5h ? "5小时" : "7天";
  const expectedRemaining = Math.min(100, Math.max(0, (remainingMillis / totalMillis) * 100));
  const actualRemaining = Math.min(100, Math.max(0, bucket.remaining_percent || 0));
  const used = Math.min(100, Math.max(0, 100 - actualRemaining));
  const expectedUsed = Math.min(100, Math.max(0, 100 - expectedRemaining));
  const delta = actualRemaining - expectedRemaining;
  const isOver = delta < -1.0;
  const deltaAbsStr = Math.abs(delta).toFixed(1) + "%";
  const status = isOver ? `消耗偏快 (-${deltaAbsStr})` : (Math.abs(delta) <= 1.0 ? `匀速正常 (±${deltaAbsStr})` : `节奏充裕 (+${deltaAbsStr})`);

  return {
    actualRemaining,
    expectedRemaining,
    used,
    expectedUsed,
    delta,
    isOver,
    status,
    windowLabel
  };
}

function renderPacedMetricBox(title, titleClass, bucket, tier) {
  const pace = calculateQuotaPace(title, bucket, tier);
  const remaining = bucket.remaining_percent || 0;
  const width = Math.min(100, Math.max(0, remaining));
  const statusClass = getQuotaStatusClass(remaining);
  let cursorHtml = "";
  let paceTooltip = "";
  if (pace) {
    paceTooltip = `\n• 消耗节奏：${pace.status}\n• 已使用：${pace.used.toFixed(1)}% (${pace.windowLabel}匀速应耗 ${pace.expectedUsed.toFixed(1)}%)\n• 理论应剩：${pace.expectedRemaining.toFixed(1)}% (白色游标位置)`;
    cursorHtml = `<div class="weekly-pace-cursor ${pace.isOver ? "over-pacing" : ""}" style="left: ${pace.expectedRemaining.toFixed(1)}%;" title="${pace.windowLabel}匀速基准线: ${pace.expectedRemaining.toFixed(1)}%"></div>`;
  }
  const resetText = getQuotaResetDisplayText(bucket);
  const fullTitle = `${resetText}${paceTooltip}`;

  return `
    <div class="metric-box metric-box-weekly" title="${escapeHtml(fullTitle)}">
      <div class="metric-box-header">
        <span class="metric-box-title ${titleClass}">${escapeHtml(title)}</span>
        <span class="metric-box-value ${statusClass}">${remaining.toFixed(1)}%</span>
      </div>
      <div class="metric-mini-track weekly-track">
        <div class="metric-mini-fill ${statusClass}" style="width: ${width}%;"></div>
        ${cursorHtml}
      </div>
      <span class="metric-box-reset" title="${escapeHtml(fullTitle)}">${escapeHtml(resetText)}</span>
    </div>
  `;
}

function buildAccountQuotaCard(acc, isCurrent) {
  const g5h = acc.gemini_5h || { remaining_percent: 0, reset_friendly: "未知" };
  const gWk = acc.gemini_weekly || { remaining_percent: 0, reset_friendly: "未知" };
  const c5h = acc.claude_5h || { remaining_percent: 0, reset_friendly: "未知" };
  const cWk = acc.claude_weekly || { remaining_percent: 0, reset_friendly: "未知" };
  const argon = acc.gemini_4_argon || null;

  const card = document.createElement("div");
  card.className = `quota-account-card ${isCurrent ? "current-account-card" : ""}`;

  const displayEmail = isCockpitEmailMasked ? maskEmail(acc.email) : acc.email;
  const tierBadgeHtml = getTierBadgeHtml(acc.subscription_tier);
  const switchBtnHtml = (!isCurrent && isCockpitSwitchEnabled())
    ? `<button class="quota-switch-btn" data-id="${escapeHtml(acc.id)}" data-email="${escapeHtml(acc.email)}">切换</button>`
    : "";
  const argonStatusClass = argon && typeof argon.remaining_percent === "number"
    ? (argon.remaining_percent < 20 ? "danger" : "argon")
    : "argon";
  const argonPace = argon && typeof argon.remaining_percent === "number"
    ? calculateQuotaPace("Gemini 4 Argon", argon, acc.subscription_tier)
    : null;
  let argonCursorHtml = "";
  let argonPaceTooltip = "";
  if (argonPace) {
    argonPaceTooltip = `\n• 消耗节奏：${argonPace.status}\n• 已使用：${argonPace.used.toFixed(1)}% (${argonPace.windowLabel}匀速应耗 ${argonPace.expectedUsed.toFixed(1)}%)\n• 理论应剩：${argonPace.expectedRemaining.toFixed(1)}% (白色游标位置)`;
    argonCursorHtml = `<div class="weekly-pace-cursor ${argonPace.isOver ? "over-pacing" : ""}" style="left: ${argonPace.expectedRemaining.toFixed(1)}%;" title="${argonPace.windowLabel}匀速基准线: ${argonPace.expectedRemaining.toFixed(1)}%"></div>`;
  }
  const argonResetText = argon ? getQuotaResetDisplayText(argon) : "";
  const argonFullTitle = `${argonResetText}${argonPaceTooltip}`;
  const argonHtml = argon && typeof argon.remaining_percent === "number"
    ? `
      <!-- 5. Bottom Full Width: Gemini 4 Argon (dynamically shown only when available) -->
      <div class="metric-box metric-box-full metric-box-argon" title="${escapeHtml(argonFullTitle)}">
        <div class="metric-box-header">
          <span class="metric-box-title argon">Gemini 4 Argon</span>
          <span class="metric-box-value ${argonStatusClass}">${argon.remaining_percent.toFixed(1)}%</span>
        </div>
        <div class="metric-mini-track argon-track">
          <div class="metric-mini-fill ${argonStatusClass}" style="width: ${Math.min(100, Math.max(0, argon.remaining_percent))}%;"></div>
          ${argonCursorHtml}
        </div>
        <span class="metric-box-reset" title="${escapeHtml(argonFullTitle)}">${escapeHtml(argonResetText)}</span>
      </div>`
    : "";

  const isFree = (acc.subscription_tier && acc.subscription_tier.toUpperCase() === "FREE") ||
    (!acc.claude_5h && !acc.gemini_5h);

  const metricsHtml = isFree ? `
      <!-- 1. Left: Claude Weekly -->
      ${renderPacedMetricBox("Claude Weekly", "claude", cWk, acc.subscription_tier)}

      <!-- 2. Right: Gemini Weekly -->
      ${renderPacedMetricBox("Gemini Weekly", "gemini", gWk, acc.subscription_tier)}
      ${argonHtml}
  ` : `
      <!-- 1. Left Top: Claude 5h -->
      ${renderPacedMetricBox("Claude 5h", "claude", c5h, acc.subscription_tier)}

      <!-- 2. Right Top: Gemini 5h -->
      ${renderPacedMetricBox("Gemini 5h", "gemini", g5h, acc.subscription_tier)}

      <!-- 3. Left Bottom: Claude Weekly -->
      ${renderPacedMetricBox("Claude Weekly", "claude", cWk, acc.subscription_tier)}

      <!-- 4. Right Bottom: Gemini Weekly -->
      ${renderPacedMetricBox("Gemini Weekly", "gemini", gWk, acc.subscription_tier)}
      ${argonHtml}
  `;

  card.innerHTML = `
    <div class="quota-card-header">
      <div class="quota-card-identity">
        <span class="quota-account-email" title="${escapeHtml(acc.email)}">${escapeHtml(displayEmail)}</span>
        ${tierBadgeHtml}
      </div>
      ${isCurrent ? `<span class="quota-active-tag"><i class="quota-active-dot"></i>使用中</span>` : switchBtnHtml}
    </div>

    <div class="quota-metrics-grid">
      ${metricsHtml}
    </div>
  `;

  if (!isCurrent && isCockpitSwitchEnabled()) {
    const switchBtn = card.querySelector(".quota-switch-btn");
    if (switchBtn) {
      switchBtn.addEventListener("click", (e) => {
        e.stopPropagation();
        switchCockpitAccount(acc.id, acc.email, switchBtn);
      });
    }
  }

  return card;
}

async function switchCockpitAccount(accountId, accountEmail, btn) {
  if (!accountId) return;
  const label = accountEmail || accountId;
  if (!confirm("切换到 " + label + " 将关闭并重启电脑上的 Antigravity，是否继续？")) {
    return;
  }
  const originalText = btn ? btn.textContent : "切换";
  if (btn) {
    btn.disabled = true;
    btn.textContent = "切换中...";
    btn.classList.add("switching");
  }

  const lastUpEl = document.getElementById("quota-last-updated");
  const prevSubtitle = lastUpEl ? lastUpEl.textContent : "";
  if (lastUpEl) {
    lastUpEl.textContent = "正在切换账号...";
  }

  try {
    const resp = await fetch("/api/v1/cockpit/switch", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ account_id: accountId }),
    });
    const data = await resp.json();
    if (!resp.ok || data.error) {
      throw new Error(data.error || ("HTTP " + resp.status));
    }
    if (lastUpEl) {
      lastUpEl.textContent = "切换成功，正在刷新...";
    }
    await new Promise((r) => setTimeout(r, 600));
    await fetchCockpitQuotas(false);
  } catch (err) {
    console.error("[Cockpit] Switch failed:", err);
    alert("切换账号失败: " + err.message);
    if (lastUpEl) {
      lastUpEl.textContent = prevSubtitle;
    }
    if (btn) {
      btn.disabled = false;
      btn.textContent = originalText;
      btn.classList.remove("switching");
    }
  }
}

function renderQuotaSheet(data) {
  if (!data) return;

  const footerEl = document.getElementById("quota-footer");
  if (footerEl && data.updated_at) {
    footerEl.textContent = `配额数据更新于 ${new Date(data.updated_at).toLocaleTimeString()}`;
  }

  const currentContainer = document.getElementById("quota-current-card");
  if (currentContainer) {
    currentContainer.innerHTML = "";
    if (data.current_account) {
      currentContainer.appendChild(buildAccountQuotaCard(data.current_account, true));
    }
  }

  const otherContainer = document.getElementById("quota-other-list");
  if (otherContainer) {
    otherContainer.innerHTML = "";
    const otherAccounts = (data.accounts || []).filter(
      (a) => !data.current_account || a.id !== data.current_account.id
    );
    const otherHeader = document.getElementById("quota-other-header");
    if (otherHeader) {
      otherHeader.textContent = `备用账号 (${otherAccounts.length})`;
      otherHeader.style.display = otherAccounts.length ? "" : "none";
    }
    if (data.cockpit_installed === false) {
      // Cockpit 未配置:仅有当前账号实时额度,备用账号区域给出安装提示
      if (otherHeader) {
        otherHeader.textContent = "备用账号";
        otherHeader.style.display = "";
      }
      const hint = document.createElement("div");
      hint.className = "quota-install-hint";
      hint.style.cssText = "padding:14px;border-radius:14px;background:rgba(128,128,128,.12);font-size:13px;line-height:1.5;";
      const title = document.createElement("div");
      title.style.cssText = "font-weight:600;margin-bottom:6px;";
      title.textContent = "未检测到 Cockpit Tools";
      const body = document.createElement("div");
      body.style.opacity = "0.7";
      body.textContent = "请安装 Cockpit Tools,并在网关里用 mgy cockpit 进行配置,即可享受备用账号额度总览与一键切换等功能。";
      hint.appendChild(title);
      hint.appendChild(body);
      otherContainer.appendChild(hint);
    } else if (otherAccounts.length === 0) {
      otherContainer.innerHTML = "";
    } else {
      otherAccounts.forEach((acc) => {
        otherContainer.appendChild(buildAccountQuotaCard(acc, false));
      });
    }
  }
}

function openQuotaSheet() {
  const sheet = document.getElementById("sheet-quota");
  if (sheet) {
    sheet.classList.remove("hidden");
    if (currentCockpitQuotas) {
      renderQuotaSheet(currentCockpitQuotas);
    } else {
      fetchCockpitQuotas();
    }
  }
}

function closeQuotaSheet() {
  const sheet = document.getElementById("sheet-quota");
  if (sheet) {
    sheet.classList.add("hidden");
  }
}

function initQuotaModule() {
  const statusBar = document.getElementById("quota-status-bar");
  if (statusBar) {
    statusBar.addEventListener("click", openQuotaSheet);
  }

  const closeBtn = document.getElementById("btn-quota-close");
  if (closeBtn) {
    closeBtn.addEventListener("click", closeQuotaSheet);
  }

  const sheet = document.getElementById("sheet-quota");
  if (sheet) {
    sheet.addEventListener("click", (e) => {
      if (e.target === sheet) {
        closeQuotaSheet();
      }
    });
  }

  const maskBtn = document.getElementById("btn-quota-mask");
  if (maskBtn) {
    if (isCockpitEmailMasked) {
      maskBtn.classList.add("active");
    }
    maskBtn.addEventListener("click", () => {
      isCockpitEmailMasked = !isCockpitEmailMasked;
      localStorage.setItem("cockpit_email_masked", isCockpitEmailMasked ? "true" : "false");
      maskBtn.classList.toggle("active", isCockpitEmailMasked);
      if (currentCockpitQuotas) {
        renderQuotaSheet(currentCockpitQuotas);
      }
    });
  }

  // Initial fetch
  fetchCockpitQuotas();
  fetchGlobalBackgroundTasksBar();

  // Periodic poll every 5 minutes (300,000 ms) while app is open
  setInterval(() => {
    if (document.visibilityState === "visible") {
      fetchCockpitQuotas();
    }
  }, 5 * 60 * 1000);

  setInterval(() => {
    if (document.visibilityState === "visible") {
      fetchGlobalBackgroundTasksBar();
    }
  }, 8000);
}

let cachedGlobalBackgroundTasks = [];

async function fetchGlobalBackgroundTasksBar() {
  const bar = document.getElementById("global-tasks-bar");
  const label = document.getElementById("global-tasks-label");
  if (!bar || !label) return;
  try {
    const res = await gwFetch("/gateway/tasks/background");
    if (!res.ok) return;
    const data = await res.json();
    const tasks = Array.isArray(data.tasks) ? data.tasks : [];
    cachedGlobalBackgroundTasks = tasks;
    if (tasks.length === 0) {
      bar.style.display = "none";
      return;
    }
    const daemonCount = tasks.filter(t => t.isDaemon).length;
    label.textContent = daemonCount > 0
      ? `${tasks.length} 个后台任务运行中（${daemonCount} 个常驻）`
      : `${tasks.length} 个后台任务运行中`;
    bar.style.display = "";
    bar.onclick = () => {
      const first = tasks[0];
      if (first && first.conversationId && typeof openConversation === "function") {
        openConversation(first.conversationId, first.conversationTitle || "会话详情", first.workspaceName || "");
      }
    };
  } catch { /* ignore */ }
}