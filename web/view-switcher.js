/**
 * Multigravity 屏幕自适应与品牌对齐控制器
 * 1. 响应式自适应：严格按照屏幕宽度动态感知并切换模式（Desktop 桌面端 >=1024px、Tablet 平板 768px~1023px、Mobile 手机 <768px）。
 *    - 手机窄屏下，桌面工作台侧边栏自动转为平滑抽屉（Drawer）并配合半透明遮罩，主区域满屏展示，点按遮罩或切换会话时自动收起抽屉。
 *    - 平板模式下自适应紧凑侧边栏与触控交互，无任何横向溢出。
 *    - 严禁触发任何页面强制重载（location.replace），无缝保持用户当前会话与编辑状态。
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

  function openDrawer() {
    document.documentElement.classList.add("agy-drawer-open");
    document.body.classList.add("agy-drawer-open");
  }

  function closeDrawer() {
    document.documentElement.classList.remove("agy-drawer-open");
    document.body.classList.remove("agy-drawer-open");
  }

  function toggleDrawer() {
    if (document.body.classList.contains("agy-drawer-open")) {
      closeDrawer();
    } else {
      openDrawer();
    }
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
    if (mode !== "mobile") {
      closeDrawer();
    }
  }

  let workbenchTagged = false;

  // 为桌面工作台标注关键响应式布局 DOM 节点与事件绑定
  function tagWorkbenchElements() {
    if (!isDesktopView || workbenchTagged) return;

    // 1. 标注工作台外层容器及其三个核心子元素（侧边栏、Sash分割线、主会话区域）
    const container = document.querySelector('[style*="container-type: size"]');
    if (container && container.children.length >= 3) {
      container.classList.add("agy-workbench-container");
      const sidebar = container.children[0];
      const sash = container.children[1];
      const main = container.children[2];

      if (!sidebar.classList.contains("agy-desktop-sidebar")) {
        sidebar.classList.add("agy-desktop-sidebar");
      }
      if (!sash.classList.contains("agy-desktop-sash")) {
        sash.classList.add("agy-desktop-sash");
      }
      if (!main.classList.contains("agy-desktop-main")) {
        main.classList.add("agy-desktop-main");
      }
      workbenchTagged = true;
    }

    // 2. 确保手机模式抽屉遮罩层存在
    let backdrop = document.getElementById("agy-mobile-drawer-backdrop");
    if (!backdrop) {
      backdrop = document.createElement("div");
      backdrop.id = "agy-mobile-drawer-backdrop";
      backdrop.className = "agy-mobile-drawer-backdrop";
      document.body.appendChild(backdrop);

      backdrop.addEventListener("click", () => {
        closeDrawer();
      });
      backdrop.addEventListener("touchstart", (e) => {
        e.preventDefault();
        closeDrawer();
      }, { passive: false });
    }

    // 3. 增强侧边栏切换按钮在手机模式下的抽屉控制
    const toggleBtn = document.querySelector('button[aria-label="切换侧边栏"]') ||
                      document.querySelector('button[aria-label*="侧边栏"]') ||
                      document.querySelector('button[aria-label*="sidebar" i]');
    if (toggleBtn && !toggleBtn.__agy_bound) {
      toggleBtn.__agy_bound = true;
      toggleBtn.classList.add("agy-sidebar-toggle-btn");
      toggleBtn.addEventListener("click", () => {
        const mode = getScreenMode();
        if (mode === "mobile") {
          toggleDrawer();
        }
      });
    }

    // 4. 手机模式下，点击会话列表项或新建会话自动收起侧边栏抽屉，呈现对话界面
    const sidebar = document.querySelector(".agy-desktop-sidebar");
    if (sidebar && !sidebar.__agy_click_bound) {
      sidebar.__agy_click_bound = true;
      sidebar.addEventListener("click", (e) => {
        if (getScreenMode() !== "mobile") return;
        const target = e.target;
        const clickable = target.closest("a") || target.closest("button") || target.closest('[role="button"]');
        if (clickable) {
          const text = (clickable.textContent || "").trim();
          const href = clickable.getAttribute("href");
          const isExpandArrow = clickable.getAttribute("aria-label")?.includes("选项") ||
                                clickable.getAttribute("aria-label")?.includes("创建新工程") ||
                                (clickable.querySelector("svg") && !text && !href);
          if (!isExpandArrow && (href || text.includes("新建会话") || text.includes("历史会话") || clickable.closest("li"))) {
            setTimeout(closeDrawer, 120);
          }
        }
      });
    }
  }

  // --- 2.5. 视图模式手动切换器 (Desktop ↔ Mobile View Switcher) ---
  function initSwitcher() {
    if (isDesktopView) {
      if (!document.getElementById("agy-view-switcher-desktop-btn")) {
        const btn = document.createElement("button");
        btn.id = "agy-view-switcher-desktop-btn";
        btn.className = "agy-view-switcher desktop-mode";
        btn.title = "切换至轻量移动端视图";
        btn.innerHTML = '<span class="agy-view-switcher-icon">📱</span><span>切换移动视图</span>';
        btn.addEventListener("click", () => {
          document.cookie = "agy_view_mode=mobile; path=/; max-age=31536000; SameSite=Lax";
          try { localStorage.setItem("agy_view_mode", "mobile"); } catch (e) {}
          const cascadeId = getCurrentCascadeId();
          window.location.href = "/?view=mobile" + (cascadeId ? "#c=" + cascadeId : "");
        });
        if (document.body) document.body.appendChild(btn);
      }
    } else {
      if (!document.getElementById("agy-view-switcher-btn")) {
        const btn = document.createElement("button");
        btn.id = "agy-view-switcher-btn";
        btn.className = "agy-view-switcher";
        btn.title = "切换至电脑/iPad 桌面工作台";
        btn.innerHTML = '<span class="agy-view-switcher-icon">🖥️</span><span>桌面工作台</span>';
        btn.addEventListener("click", () => {
          document.cookie = "agy_view_mode=desktop; path=/; max-age=31536000; SameSite=Lax";
          try { localStorage.setItem("agy_view_mode", "desktop"); } catch (e) {}
          let cascadeId = "";
          if (window.location.hash.startsWith("#c=")) cascadeId = window.location.hash.slice(3);
          window.location.href = cascadeId ? ("/c/" + cascadeId + "?view=desktop") : "/?view=desktop";
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

  function getCurrentCascadeId() {
    // 1. From desktop URL path /c/<id>
    const pathMatch = window.location.pathname.match(/\/c\/([a-zA-Z0-9_-]+)/);
    if (pathMatch && pathMatch[1]) return pathMatch[1];
    // 2. From hash #c=<id>
    if (window.location.hash.startsWith("#c=")) return window.location.hash.slice(3);
    return "";
  }

  function checkResponsiveViewAdaptation() {
    // Desktop and mobile are separate frontends served by different handlers.
    // Automatic navigation between them on resize is disruptive and can cause
    // reload loops near breakpoints. Instead, rely on CSS responsive rules
    // within each view mode and let the user explicitly switch via cookie/URL.
    return false;
  }

  function runAll() {
    updateScreenModeAttributes();
    initSwitcher();
    if (isDesktopView) {
      alignDesktopBrand();
      tagWorkbenchElements();
    }
  }

  // 监听窗口尺寸变化，自适应屏幕模式与视图动态切换
  let resizeTimer = null;
  window.addEventListener("resize", () => {
    updateScreenModeAttributes();
    if (isDesktopView && !workbenchTagged) tagWorkbenchElements();
    if (resizeTimer) clearTimeout(resizeTimer);
    resizeTimer = setTimeout(() => {
      resizeTimer = null;
      updateScreenModeAttributes();
      if (isDesktopView && !workbenchTagged) {
        tagWorkbenchElements();
      }
    }, 200);
  });

  // 仅在工作台容器初始挂载前进行轻量监听，一旦挂载完成立即注销 Observer，彻底消除会话切换性能损耗
  let workbenchObserver = null;
  function startWorkbenchObserver() {
    if (!isDesktopView || workbenchTagged || workbenchObserver) return;
    workbenchObserver = new MutationObserver(() => {
      tagWorkbenchElements();
      alignDesktopBrand();
      if (workbenchTagged && workbenchObserver) {
        workbenchObserver.disconnect();
        workbenchObserver = null;
      }
    });
    if (document.body) {
      workbenchObserver.observe(document.body, { childList: true, subtree: true });
    }
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", () => {
      runAll();
      startWorkbenchObserver();
    });
  } else {
    runAll();
    startWorkbenchObserver();
  }
})();
