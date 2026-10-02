/**
 * Multigravity 屏幕自适应与品牌对齐控制器
 * 1. 响应式自适应：严格按照屏幕宽度自适应不同模式（Desktop 桌面端、Tablet 平板、Mobile 手机），无手动切换按钮。
 * 2. 品牌与标题对齐：桌面工作台对齐为 Multigravity，锁定 Favicon 与标题。
 * 3. 授权状态感知：桌面端未授权时提供一键配对接入。
 */
(function () {
  "use strict";

  const isDesktopView = window.__APP_CONFIG__ !== undefined;

  // --- 1. 桌面工作台品牌对齐 (Multigravity Brand Alignment) ---
  function alignDesktopBrand() {
    if (document.title && !document.title.includes("Multigravity")) {
      document.title = "Multigravity";
    }

    const existingLogos = document.querySelectorAll(".multigravity-brand-logo");
    for (let i = 0; i < existingLogos.length; i++) {
      existingLogos[i].remove();
    }

    const brandCandidates = document.querySelectorAll(".font-semibold.text-sm");
    for (let i = 0; i < brandCandidates.length; i++) {
      const el = brandCandidates[i];
      const text = (el.textContent || "").trim();
      if (text === "Antigravity" || text === "Multigravity") {
        if (text !== "Multigravity") {
          el.textContent = "Multigravity";
        }
        if (el.style.paddingLeft === "0px" || el.style.paddingLeft === "0") {
          el.style.paddingLeft = "";
        }
      }
    }

    const favicons = document.querySelectorAll('link[rel*="icon"]');
    for (let i = 0; i < favicons.length; i++) {
      const fav = favicons[i];
      if (fav.href && (fav.href.includes("data:image/svg+xml") || fav.href.includes("%F0%9F%8E%81") || fav.href.includes("🎁"))) {
        fav.type = "image/x-icon";
        fav.href = "/favicon.ico?v=3";
      }
    }
  }

  // --- 2. 屏幕宽度自适应控制器 (Desktop / 平板 / 手机) ---
  const BREAKPOINT_TABLET = 768;
  const BREAKPOINT_DESKTOP = 1024;

  function getScreenMode() {
    const w = window.innerWidth;
    if (w < BREAKPOINT_TABLET) return "mobile";
    if (w < BREAKPOINT_DESKTOP) return "tablet";
    return "desktop";
  }

  function updateScreenModeAttributes() {
    const mode = getScreenMode();
    const targets = [document.documentElement, document.body].filter(Boolean);
    for (let i = 0; i < targets.length; i++) {
      const el = targets[i];
      el.setAttribute("data-screen-mode", mode);
      el.classList.remove("mode-desktop", "mode-tablet", "mode-mobile");
      el.classList.add("mode-" + mode);
    }
  }

  // 清除历史版本遗留的切换按钮与横幅
  function cleanupLegacyButtons() {
    const legacyIds = [
      "agy-view-switcher-btn",
      "agy-view-switcher-desktop-btn",
      "agy-wide-mobile-banner"
    ];
    for (let i = 0; i < legacyIds.length; i++) {
      const el = document.getElementById(legacyIds[i]);
      if (el) el.remove();
    }
  }

  function checkResponsiveViewAdaptation() {
    const w = window.innerWidth;
    const isMobileWidth = w < BREAKPOINT_TABLET;

    if (isDesktopView && isMobileWidth) {
      // 桌面端视图下若检测到手机窄屏，自适应切至移动端视图
      document.cookie = "agy_view_mode=mobile; path=/; max-age=31536000; SameSite=Lax";
      try { localStorage.setItem("agy_view_mode", "mobile"); } catch (e) {}
      window.location.replace("/?view=mobile");
      return true;
    }

    if (!isDesktopView && !isMobileWidth) {
      // 移动端视图下若检测到平板/桌面宽屏，自适应切至桌面工作台
      document.cookie = "agy_view_mode=desktop; path=/; max-age=31536000; SameSite=Lax";
      try { localStorage.setItem("agy_view_mode", "desktop"); } catch (e) {}
      window.location.replace("/?view=desktop");
      return true;
    }

    return false;
  }

  // --- 3. 桌面工作台未配对弹窗 ---
  function showDesktopPairModal(errMsg) {
    if (document.getElementById("agy-desktop-pair-modal")) {
      const errEl = document.getElementById("agy-desktop-pair-err");
      if (errEl && errMsg) {
        errEl.textContent = errMsg;
        errEl.style.display = "block";
      }
      return;
    }

    const overlay = document.createElement("div");
    overlay.id = "agy-desktop-pair-modal";
    overlay.className = "agy-desktop-pair-overlay";
    overlay.innerHTML = `
      <div class="agy-desktop-pair-card">
        <h2>Multigravity 桌面工作台</h2>
        <p>当前设备尚未配对授权。请在服务端终端执行 <code>mgy pair</code> 并输入配对码以连接。</p>
        <input type="text" id="agy-desktop-pair-input" class="agy-desktop-pair-input" placeholder="输入配对码或粘贴配对链接..." autofocus autocomplete="off" />
        <button id="agy-desktop-pair-btn" class="agy-desktop-pair-btn">确认配对并进入工作台</button>
        <div id="agy-desktop-pair-err" class="agy-desktop-pair-err" style="${errMsg ? 'display:block;' : 'display:none;'}">${errMsg || ''}</div>
      </div>
    `;

    document.body.appendChild(overlay);

    const input = document.getElementById("agy-desktop-pair-input");
    const btn = document.getElementById("agy-desktop-pair-btn");
    const errEl = document.getElementById("agy-desktop-pair-err");

    async function doPair() {
      let code = (input.value || "").trim();
      if (!code) {
        errEl.textContent = "请输入有效的配对码";
        errEl.style.display = "block";
        return;
      }
      if (code.includes("code=")) {
        const match = code.match(/code=([a-zA-Z0-9]+)/);
        if (match) code = match[1];
      }
      btn.disabled = true;
      btn.textContent = "正在验证...";
      errEl.style.display = "none";

      try {
        const resp = await fetch("/api/v1/auth/pair", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            pairing_code: code,
            device_name: "Web Desktop",
            platform: "desktop"
          })
        });
        const data = await resp.json();
        if (!resp.ok) {
          throw new Error(data.error || "配对失败，请检查配对码是否正确");
        }
        btn.textContent = "配对成功，正在载入...";
        try {
          localStorage.setItem("agy_paired", "1");
          localStorage.setItem("agy_device_id", data.device_id || "desktop");
          localStorage.setItem("agy_view_mode", "desktop");
          document.cookie = "agy_view_mode=desktop; path=/; max-age=31536000";
        } catch (_) {}
        setTimeout(() => window.location.reload(), 400);
      } catch (err) {
        errEl.textContent = err.message || "配对失败";
        errEl.style.display = "block";
        btn.disabled = false;
        btn.textContent = "确认配对并进入工作台";
      }
    }

    btn.addEventListener("click", doPair);
    input.addEventListener("keydown", (e) => {
      if (e.key === "Enter") doPair();
    });
    setTimeout(() => input.focus(), 100);
  }

  // 监听桌面端 401 响应以触发配对
  if (isDesktopView) {
    const origFetch = window.fetch;
    window.fetch = async function (...args) {
      const resp = await origFetch.apply(this, args);
      const url = typeof args[0] === "string" ? args[0] : (args[0] && args[0].url) || "";
      if (resp.status === 401 && !url.includes("/api/v1/auth/pair")) {
        showDesktopPairModal("当前设备未授权或凭据已过期，请输入配对码重新连接");
      }
      return resp;
    };

    // 检查初始认证状态
    if (!localStorage.getItem("agy_paired")) {
      fetch("/gateway/status").then(r => {
        if (r.status === 401) {
          showDesktopPairModal();
        }
      }).catch(() => {});
    }
  }

  function runAll() {
    cleanupLegacyButtons();
    updateScreenModeAttributes();
    checkResponsiveViewAdaptation();
    if (isDesktopView) alignDesktopBrand();
  }

  // 监听窗口尺寸变化，自适应切换屏幕模式（防抖避免频繁重定向）
  let resizeTimer = null;
  window.addEventListener("resize", () => {
    updateScreenModeAttributes();
    if (resizeTimer) clearTimeout(resizeTimer);
    resizeTimer = setTimeout(() => {
      resizeTimer = null;
      checkResponsiveViewAdaptation();
    }, 250);
  });

  let brandTimer = null;
  const debouncedAlignBrand = () => {
    if (brandTimer) return;
    brandTimer = requestAnimationFrame(() => {
      brandTimer = null;
      if (isDesktopView) alignDesktopBrand();
    });
  };

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", () => {
      runAll();
      if (document.body && isDesktopView) {
        new MutationObserver(debouncedAlignBrand).observe(document.body, {
          childList: true,
          subtree: true,
        });
      }
    });
  } else {
    runAll();
    if (document.body && isDesktopView) {
      new MutationObserver(debouncedAlignBrand).observe(document.body, {
        childList: true,
        subtree: true,
      });
    }
  }
})();
