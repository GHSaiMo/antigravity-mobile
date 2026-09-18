/**
 * Multigravity Mobile / Desktop 双模视图与品牌对齐控制器
 * 1. 桌面工作台/宽屏/iPad：自动将品牌名称与 Logo 对齐为 Multigravity，锁定 Favicon 与标题。
 * 2. 窄屏移动版：保留便捷切换到桌面工作台的悬浮入口。
 */
(function () {
  "use strict";

  // --- 1. 桌面工作台品牌对齐 (Multigravity Brand Alignment) ---
  function alignDesktopBrand() {
    // 1.1 锁定页面标题
    if (document.title && !document.title.includes("Multigravity")) {
      document.title = "Multigravity";
    }

    // 1.2 检查顶部左侧品牌文字元素
    // 在 main.js 中，品牌文字为：<span class="font-semibold text-sm shrink-0 pl-2 pr-1.5 mr-1">Multigravity</span>
    const brandCandidates = document.querySelectorAll(".font-semibold.text-sm, span.shrink-0");
    for (let i = 0; i < brandCandidates.length; i++) {
      const el = brandCandidates[i];
      const text = (el.textContent || "").trim();
      if (text === "Antigravity" || text === "Multigravity") {
        if (text !== "Multigravity") {
          el.textContent = "Multigravity";
        }
        // 检查是否已有品牌 Logo
        const prev = el.previousElementSibling;
        if (!prev || !prev.classList.contains("multigravity-brand-logo")) {
          const img = document.createElement("img");
          img.src = "/icons/icon-192.png";
          img.alt = "Multigravity";
          img.className = "multigravity-brand-logo";
          img.title = "Multigravity";
          if (el.parentElement) {
            el.parentElement.insertBefore(img, el);
            el.style.paddingLeft = "0";
          }
        }
      }
    }

    // 1.3 兜底替换任何可能渲染出来的旧版 Antigravity 三角形 SVG (viewBox 0 0 180 180)
    const oldIcons = document.querySelectorAll('svg[viewBox="0 0 180 180"]');
    for (let i = 0; i < oldIcons.length; i++) {
      const svg = oldIcons[i];
      if (svg.dataset.replacedByM) continue;
      svg.dataset.replacedByM = "true";
      const img = document.createElement("img");
      img.src = "/icons/icon-192.png";
      img.alt = "Multigravity";
      img.className = "multigravity-brand-logo";
      const w = svg.getAttribute("width") || svg.clientWidth || 22;
      const h = svg.getAttribute("height") || svg.clientHeight || 22;
      img.style.width = w + "px";
      img.style.height = h + "px";
      if (svg.parentElement) {
        svg.parentElement.replaceChild(img, svg);
      }
    }
  }

  // --- 2. 移动端视图切换浮钮 ---
  function initSwitcher() {
    const isDesktopView = window.__APP_CONFIG__ !== undefined;
    if (isDesktopView || window.innerWidth >= 768) {
      return;
    }
    if (document.getElementById("agy-view-switcher-btn")) return;

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

    if (document.body) {
      document.body.appendChild(btn);
    }
  }

  function runAll() {
    initSwitcher();
    alignDesktopBrand();
  }

  // 启动观察与周期监听
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", () => {
      runAll();
      if (document.body) {
        new MutationObserver(alignDesktopBrand).observe(document.body, {
          childList: true,
          subtree: true,
          characterData: true
        });
      }
    });
  } else {
    runAll();
    if (document.body) {
      new MutationObserver(alignDesktopBrand).observe(document.body, {
        childList: true,
        subtree: true,
        characterData: true
      });
    }
  }

  // 定时兜底（处理 React 路由与重渲染）
  setInterval(alignDesktopBrand, 600);
})();
