/**
 * Multigravity Mobile / Desktop 双模视图与品牌对齐控制器
 * 1. 桌面工作台/宽屏/iPad：自动将品牌名称与 Logo 对齐为 Multigravity，锁定 Favicon 与标题。
 * 2. 移动端：常驻提供便捷切换到桌面工作台的悬浮入口与宽屏横幅提示。
 * 3. 桌面端：支持一键切回移动视图，并在未配对时弹出桌面专属配对弹窗。
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

  // --- 2. 视图切换控制器 ---
  function initSwitcher() {
    if (!isDesktopView) {
      // 移动视图：提供切换至桌面工作台
      if (!document.getElementById("agy-view-switcher-btn")) {
        const btn = document.createElement("button");
        btn.id = "agy-view-switcher-btn";
        btn.className = "agy-view-switcher";
        btn.title = "切换到电脑/iPad 桌面工作台";
        btn.innerHTML = '<span class="agy-view-switcher-icon">🖥️</span><span>桌面工作台</span>';
        btn.addEventListener("click", () => {
          document.cookie = "agy_view_mode=desktop; path=/; max-age=31536000";
          try { localStorage.setItem("agy_view_mode", "desktop"); } catch (e) {}
          window.location.href = "/?view=desktop";
        });
        if (document.body) document.body.appendChild(btn);
      }

      // 宽屏时，在移动版顶部增加醒目横幅提示
      if (window.innerWidth >= 768 && !document.getElementById("agy-wide-mobile-banner")) {
        const banner = document.createElement("div");
        banner.id = "agy-wide-mobile-banner";
        banner.className = "agy-wide-mobile-banner";
        banner.innerHTML = `
          <span>💻 检测到当前为宽屏设备，推荐使用完整的 <strong>桌面工作台</strong> 体验</span>
          <button class="agy-wide-mobile-banner-btn" id="agy-wide-mobile-switch-btn">立即切换 🖥️</button>
        `;
        if (document.body) {
          document.body.prepend(banner);
          document.getElementById("agy-wide-mobile-switch-btn")?.addEventListener("click", () => {
            document.cookie = "agy_view_mode=desktop; path=/; max-age=31536000";
            try { localStorage.setItem("agy_view_mode", "desktop"); } catch (e) {}
            window.location.href = "/?view=desktop";
          });
        }
      }
    } else {
      // 桌面视图：在左下角提供低调的切回移动端入口
      if (!document.getElementById("agy-view-switcher-desktop-btn")) {
        const btn = document.createElement("button");
        btn.id = "agy-view-switcher-desktop-btn";
        btn.className = "agy-view-switcher desktop-mode";
        btn.title = "切换至轻量移动端视图";
        btn.innerHTML = '<span class="agy-view-switcher-icon">📱</span><span>移动端视图</span>';
        btn.addEventListener("click", () => {
          document.cookie = "agy_view_mode=mobile; path=/; max-age=31536000";
          try { localStorage.setItem("agy_view_mode", "mobile"); } catch (e) {}
          window.location.href = "/?view=mobile";
        });
        if (document.body) document.body.appendChild(btn);
      }
    }
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
    initSwitcher();
    if (isDesktopView) alignDesktopBrand();
  }

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
