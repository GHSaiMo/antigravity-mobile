/**
 * Antigravity Mobile / Desktop 双模视图自由切换控制器
 */
(function () {
  'use strict';

  function initSwitcher() {
    if (document.getElementById('agy-view-switcher-btn')) return;

    // 检查当前所处视图
    const isDesktopView = window.__APP_CONFIG__ !== undefined;

    const btn = document.createElement('button');
    btn.id = 'agy-view-switcher-btn';
    btn.className = 'agy-view-switcher';

    if (isDesktopView) {
      btn.title = '切换到极简手机版 PWA';
      btn.innerHTML = '<span class="agy-view-switcher-icon">📱</span><span>极简手机版</span>';
      btn.addEventListener('click', () => {
        document.cookie = 'agy_view_mode=mobile; path=/; max-age=31536000';
        try { localStorage.setItem('agy_view_mode', 'mobile'); } catch (e) {}
        window.location.href = '/?view=mobile';
      });
    } else {
      btn.title = '切换到电脑/iPad 桌面工作台';
      btn.innerHTML = '<span class="agy-view-switcher-icon">🖥️</span><span>桌面工作台</span>';
      btn.addEventListener('click', () => {
        document.cookie = 'agy_view_mode=desktop; path=/; max-age=31536000';
        try { localStorage.setItem('agy_view_mode', 'desktop'); } catch (e) {}
        window.location.href = '/?view=desktop';
      });
    }

    document.body.appendChild(btn);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initSwitcher);
  } else {
    initSwitcher();
  }
})();
