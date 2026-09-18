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

    // 1.2 宽屏版左上角不保留 logo，彻底清除任何注入或遗留的品牌 logo 节点
    const existingLogos = document.querySelectorAll(".multigravity-brand-logo");
    for (let i = 0; i < existingLogos.length; i++) {
      existingLogos[i].remove();
    }

    // 1.3 确保顶部左侧品牌文字元素显示为 Multigravity，并只保留文字
    // 在桌面工作台中，品牌文字节点为：<span class="font-semibold text-sm shrink-0 pl-2 pr-1.5 mr-1">Multigravity</span>
    const brandCandidates = document.querySelectorAll(".font-semibold.text-sm");
    for (let i = 0; i < brandCandidates.length; i++) {
      const el = brandCandidates[i];
      const text = (el.textContent || "").trim();
      if (text === "Antigravity" || text === "Multigravity") {
        if (text !== "Multigravity") {
          el.textContent = "Multigravity";
        }
        // 恢复原生内边距样式（清除此前为插入 logo 所设置的 style.paddingLeft = 0）
        if (el.style.paddingLeft === "0px" || el.style.paddingLeft === "0") {
          el.style.paddingLeft = "";
        }
      }
    }

    // 1.4 确保所有 Favicon 链接均指向透明通道的高清图标与 favicon.ico
    const favicons = document.querySelectorAll('link[rel*="icon"]');
    for (let i = 0; i < favicons.length; i++) {
      const fav = favicons[i];
      if (fav.href && (fav.href.includes("data:image/svg+xml") || fav.href.includes("%F0%9F%8E%81") || fav.href.includes("🎁"))) {
        fav.type = "image/x-icon";
        fav.href = "/favicon.ico?v=3";
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
})();
