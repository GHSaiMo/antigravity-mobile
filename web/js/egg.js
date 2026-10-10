// 彩蛋：连点列表页大标题 "Multigravity" 10 次（相邻两次间隔 ≤ 2 秒），对齐 iOS / Android。
// 卡片 + 五组烟花，约 2.3 秒后自动淡出。

(function () {
  const TAPS_NEEDED = 10;
  const TAP_GAP_MS = 2000;
  const COLORS = ["#FFD700", "#A855F7", "#06B6D4", "#F43F5E", "#10B981", "#FF9800", "#FFFFFF"];
  let tapCount = 0;
  let lastTapAt = 0;
  let showing = false;

  function rand(min, max) {
    return min + Math.random() * (max - min);
  }

  function makeBurst(x, y, delay, count) {
    const particles = [];
    for (let i = 0; i < count; i++) {
      particles.push({
        angle: (i / count) * Math.PI * 2 + rand(-0.15, 0.15),
        speed: rand(80, 240),
        color: COLORS[Math.floor(Math.random() * COLORS.length)],
        size: rand(4, 9),
        alpha: rand(0.85, 1),
      });
    }
    return { x, y, delay, duration: 1.5, particles };
  }

  function runFireworks(canvas) {
    const dpr = window.devicePixelRatio || 1;
    const w = window.innerWidth;
    const h = window.innerHeight;
    canvas.width = w * dpr;
    canvas.height = h * dpr;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    ctx.scale(dpr, dpr);
    const cx = w / 2;
    const cy = h / 2;
    const bursts = [
      makeBurst(cx - 80, cy - 90, 0, 30),
      makeBurst(cx + 85, cy - 75, 0.15, 32),
      makeBurst(cx - 75, cy + 70, 0.35, 28),
      makeBurst(cx + 80, cy + 85, 0.5, 30),
      makeBurst(cx, cy - 120, 0.75, 36),
    ];
    const start = performance.now();
    function frame(now) {
      if (!canvas.isConnected) return;
      const elapsed = (now - start) / 1000;
      ctx.clearRect(0, 0, w, h);
      let alive = false;
      for (const b of bursts) {
        const t = elapsed - b.delay;
        if (t >= b.duration) continue;
        alive = true;
        if (t <= 0) continue;
        const p = t / b.duration;
        const ease = 1 - Math.pow(1 - p, 3);
        const gravity = p * p * 35;
        for (const q of b.particles) {
          const px = b.x + Math.cos(q.angle) * q.speed * ease;
          const py = b.y + Math.sin(q.angle) * q.speed * ease + gravity;
          const a = q.alpha * Math.max(0, 1 - p);
          const size = q.size * Math.max(0.3, 1 - p * 0.6);
          ctx.globalAlpha = a;
          ctx.fillStyle = q.color;
          ctx.beginPath();
          ctx.arc(px, py, size / 2, 0, Math.PI * 2);
          ctx.fill();
          if (q.size > 6 && p < 0.6) {
            ctx.globalAlpha = a * 0.8;
            ctx.fillStyle = "#fff";
            ctx.beginPath();
            ctx.arc(px, py, size / 4, 0, Math.PI * 2);
            ctx.fill();
          }
        }
      }
      ctx.globalAlpha = 1;
      if (alive) requestAnimationFrame(frame);
    }
    requestAnimationFrame(frame);
  }

  function showEasterEgg() {
    if (showing) return;
    showing = true;
    const overlay = document.createElement("div");
    overlay.className = "egg-overlay";
    overlay.innerHTML = `
      <canvas class="egg-canvas"></canvas>
      <div class="egg-card">
        <img class="egg-logo" src="/icons/icon-192.png" alt="" />
        <div class="egg-title">Multigravity</div>
        <div class="egg-credit">✨ Design by Jiuge</div>
      </div>`;
    document.body.appendChild(overlay);
    if (!window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
      runFireworks(overlay.querySelector(".egg-canvas"));
    }
    requestAnimationFrame(() => overlay.classList.add("in"));
    setTimeout(() => overlay.classList.remove("in"), 2300);
    setTimeout(() => {
      overlay.remove();
      showing = false;
    }, 2900);
  }

  function onTap() {
    if (showing) return;
    const now = Date.now();
    tapCount = now - lastTapAt > TAP_GAP_MS ? 1 : tapCount + 1;
    lastTapAt = now;
    if (typeof triggerHaptic === "function") triggerHaptic("light");
    if (tapCount >= TAPS_NEEDED) {
      tapCount = 0;
      if (typeof triggerHaptic === "function") triggerHaptic("success");
      showEasterEgg();
    }
  }

  document.addEventListener("click", (e) => {
    if (e.target.closest(".large-title, .nav-brand-title")) onTap();
  });
})();
