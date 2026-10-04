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
  if (percent > 50) return "good";
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
  if (!current || !current.gemini_5h) return;

  const percentEl = document.getElementById("quota-status-percent");
  const fillEl = document.getElementById("quota-status-fill");
  const descEl = document.getElementById("quota-status-desc");

  const pct = current.gemini_5h.remaining_percent;
  const statusClass = getQuotaStatusClass(pct);

  if (percentEl) percentEl.textContent = `${pct.toFixed(1)}%`;
  if (fillEl) {
    fillEl.style.width = `${Math.min(100, Math.max(0, pct))}%`;
    fillEl.className = `quota-progress-fill fill-${statusClass}`;
  }
  if (descEl) {
    const resetTxt = current.gemini_5h.reset_friendly || "就绪";
    let displayText = resetTxt;
    if (resetTxt !== "就绪" && resetTxt !== "已就绪" && resetTxt !== "未知") {
      const dateTime = formatResetDateTime(current.gemini_5h.reset_time);
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

function buildAccountQuotaCard(acc, isCurrent) {
  const g5h = acc.gemini_5h || { remaining_percent: 0, reset_friendly: "未知" };
  const gWk = acc.gemini_weekly || { remaining_percent: 0, reset_friendly: "未知" };
  const c5h = acc.claude_5h || { remaining_percent: 0, reset_friendly: "未知" };
  const cWk = acc.claude_weekly || { remaining_percent: 0, reset_friendly: "未知" };

  const card = document.createElement("div");
  card.className = `quota-account-card ${isCurrent ? "current-account-card" : ""}`;

  const displayEmail = isCockpitEmailMasked ? maskEmail(acc.email) : acc.email;
  const switchBtnHtml = (!isCurrent && isCockpitSwitchEnabled())
    ? `<button class="quota-switch-btn" data-id="${escapeHtml(acc.id)}" data-email="${escapeHtml(acc.email)}">切换</button>`
    : "";

  card.innerHTML = `
    <div class="quota-card-header">
      <div class="quota-card-identity">
        <span class="quota-account-email" title="${escapeHtml(acc.email)}">${escapeHtml(displayEmail)}</span>
      </div>
      ${isCurrent ? `<span class="quota-active-tag">🟢 使用中</span>` : switchBtnHtml}
    </div>

    <div class="quota-metrics-grid">
      <!-- 1. Left Top: Claude 5h -->
      <div class="metric-box">
        <div class="metric-box-header">
          <span class="metric-box-title claude">🟣 Claude 5h</span>
          <span class="metric-box-value ${getQuotaStatusClass(c5h.remaining_percent)}">${c5h.remaining_percent.toFixed(1)}%</span>
        </div>
        <div class="metric-mini-track">
          <div class="metric-mini-fill ${getQuotaStatusClass(c5h.remaining_percent)}" style="width: ${Math.min(100, Math.max(0, c5h.remaining_percent))}%;"></div>
        </div>
        <span class="metric-box-reset" title="${escapeHtml(getQuotaResetDisplayText(c5h))}">${escapeHtml(getQuotaResetDisplayText(c5h))}</span>
      </div>

      <!-- 2. Right Top: Gemini 5h -->
      <div class="metric-box">
        <div class="metric-box-header">
          <span class="metric-box-title gemini">🔵 Gemini 5h</span>
          <span class="metric-box-value ${getQuotaStatusClass(g5h.remaining_percent)}">${g5h.remaining_percent.toFixed(1)}%</span>
        </div>
        <div class="metric-mini-track">
          <div class="metric-mini-fill ${getQuotaStatusClass(g5h.remaining_percent)}" style="width: ${Math.min(100, Math.max(0, g5h.remaining_percent))}%;"></div>
        </div>
        <span class="metric-box-reset" title="${escapeHtml(getQuotaResetDisplayText(g5h))}">${escapeHtml(getQuotaResetDisplayText(g5h))}</span>
      </div>

      <!-- 3. Left Bottom: Claude Weekly -->
      <div class="metric-box">
        <div class="metric-box-header">
          <span class="metric-box-title claude">🟣 Claude Weekly</span>
          <span class="metric-box-value ${getQuotaStatusClass(cWk.remaining_percent)}">${cWk.remaining_percent.toFixed(1)}%</span>
        </div>
        <div class="metric-mini-track">
          <div class="metric-mini-fill ${getQuotaStatusClass(cWk.remaining_percent)}" style="width: ${Math.min(100, Math.max(0, cWk.remaining_percent))}%;"></div>
        </div>
        <span class="metric-box-reset" title="${escapeHtml(getQuotaResetDisplayText(cWk))}">${escapeHtml(getQuotaResetDisplayText(cWk))}</span>
      </div>

      <!-- 4. Right Bottom: Gemini Weekly -->
      <div class="metric-box">
        <div class="metric-box-header">
          <span class="metric-box-title gemini">🔵 Gemini Weekly</span>
          <span class="metric-box-value ${getQuotaStatusClass(gWk.remaining_percent)}">${gWk.remaining_percent.toFixed(1)}%</span>
        </div>
        <div class="metric-mini-track">
          <div class="metric-mini-fill ${getQuotaStatusClass(gWk.remaining_percent)}" style="width: ${Math.min(100, Math.max(0, gWk.remaining_percent))}%;"></div>
        </div>
        <span class="metric-box-reset" title="${escapeHtml(getQuotaResetDisplayText(gWk))}">${escapeHtml(getQuotaResetDisplayText(gWk))}</span>
      </div>
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

  const lastUpEl = document.getElementById("quota-last-updated");
  if (lastUpEl && data.updated_at) {
    const dt = new Date(data.updated_at);
    lastUpEl.textContent = `更新于 ${dt.toLocaleTimeString()}`;
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
    if (otherAccounts.length === 0) {
      otherContainer.innerHTML = `<div style="text-align:center;color:var(--ios-tertiary-label);padding:16px;">无其他备用账号</div>`;
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

  // Periodic poll every 5 minutes (300,000 ms) while app is open
  setInterval(() => {
    if (document.visibilityState === "visible") {
      fetchCockpitQuotas();
    }
  }, 5 * 60 * 1000);
}