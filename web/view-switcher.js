/**
 * Antigravity Mobile / Desktop 双模视图切换控制器
 * 宽屏/桌面版模式不展示悬浮胶囊，仅在窄屏移动版保留切换入口。
 */
(function () {
  "use strict";

  function initSwitcher() {
    if (document.getElementById("agy-view-switcher-btn")) return;

    // 检查当前所处视图：桌面工作台或宽屏下均不显示极简手机版切换按钮
    const isDesktopView = window.__APP_CONFIG__ !== undefined;
    if (isDesktopView || window.innerWidth >= 768) {
      return;
    }

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

    document.body.appendChild(btn);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", initSwitcher);
  } else {
    initSwitcher();
  }
})();
