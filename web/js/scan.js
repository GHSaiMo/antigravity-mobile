// 扫码配对。
//
// 三级降级：
//   1. 实时摄像头（仅 HTTPS / localhost：浏览器规定 getUserMedia 需要安全上下文）
//   2. 拍照 / 从相册选图识别（<input type=file>，任何环境可用，包括局域网 http）
//   3. 手动输入配对码（配对弹层本身）
//
// 二维码内容是网关打印的 agy://pair?code=...&host=...；Web 只取 code，并且只会提交给当前站点自己的
// /api/v1/auth/pair，二维码里的 host 被忽略，所以伪造的二维码无法把配对码送到别的服务器。

const PAIR_QR = {
  stream: null,
  timer: 0,
  detector: null,
  active: false,
  decoding: false,
  jsqrPromise: null,
};

function canUseLiveCamera() {
  return !!(window.isSecureContext && navigator.mediaDevices && typeof navigator.mediaDevices.getUserMedia === "function");
}

function loadJsQR() {
  if (window.jsQR) return Promise.resolve(window.jsQR);
  if (PAIR_QR.jsqrPromise) return PAIR_QR.jsqrPromise;
  PAIR_QR.jsqrPromise = new Promise((resolve, reject) => {
    const s = document.createElement("script");
    s.src = "/js/vendor/jsQR.js?v=1";
    s.onload = () => (window.jsQR ? resolve(window.jsQR) : reject(new Error("二维码识别组件加载失败")));
    s.onerror = () => {
      PAIR_QR.jsqrPromise = null;
      reject(new Error("二维码识别组件加载失败"));
    };
    document.head.appendChild(s);
  });
  return PAIR_QR.jsqrPromise;
}

// 从二维码文本里取出配对码。返回 { code } 或 { error }。
function parseScannedPairing(text) {
  const raw = String(text || "").trim();
  if (!raw) return { error: "二维码内容为空" };
  if (raw.startsWith("agy://pair")) {
    let code = "";
    try {
      code = new URL(raw.replace("agy://", "http://")).searchParams.get("code") || "";
    } catch (_) {
      const m = raw.match(/[?&]code=([A-Za-z0-9]+)/);
      code = m ? m[1] : "";
    }
    return code ? { code } : { error: "配对二维码里没有配对码" };
  }
  // 网关的配对码是 64 位十六进制；允许直接扫描只含配对码的二维码
  if (/^[A-Fa-f0-9]{16,}$/.test(raw)) return { code: raw };
  return { error: "这不是 Multigravity 的配对二维码" };
}

function scanSetStatus(text, isError = false) {
  const el = document.getElementById("scan-status");
  if (!el) return;
  el.textContent = text || "";
  el.classList.toggle("error", !!isError);
}

function scanShowFallbackActions(show) {
  document.getElementById("scan-actions")?.classList.toggle("hidden", !show);
}

function stopLiveCamera() {
  PAIR_QR.active = false;
  if (PAIR_QR.timer) {
    clearTimeout(PAIR_QR.timer);
    PAIR_QR.timer = 0;
  }
  if (PAIR_QR.stream) {
    PAIR_QR.stream.getTracks().forEach((t) => t.stop());
    PAIR_QR.stream = null;
  }
  const video = document.getElementById("scan-video");
  if (video) video.srcObject = null;
}

function closePairingScan() {
  stopLiveCamera();
  document.getElementById("scan-overlay")?.classList.add("hidden");
}

function finishPairingScan(code) {
  closePairingScan();
  if (typeof triggerHaptic === "function") triggerHaptic("light");
  if (navigator.vibrate) navigator.vibrate(30);
  const inputEl = document.getElementById("input-pairing-code");
  const errEl = document.getElementById("pairing-error-msg");
  if (inputEl) inputEl.value = code;
  if (typeof isDevicePaired === "function" && isDevicePaired()) {
    // 已配对设备：不静默替换现有凭据，交给用户确认
    if (errEl) {
      errEl.textContent = "已扫描到配对码。当前设备已配对，点「确认配对」才会替换现有凭据。";
      errEl.classList.remove("hidden");
    }
    return;
  }
  if (typeof submitPairing === "function") submitPairing();
}

// 把画面缩到 maxEdge 以内再识别：太大的照片既慢又更容易失败。
function decodeFromCanvasSource(source, srcW, srcH, maxEdge, invert) {
  const scale = Math.min(1, maxEdge / Math.max(srcW, srcH));
  const w = Math.max(1, Math.round(srcW * scale));
  const h = Math.max(1, Math.round(srcH * scale));
  const canvas = document.createElement("canvas");
  canvas.width = w;
  canvas.height = h;
  const ctx = canvas.getContext("2d", { willReadFrequently: true });
  ctx.drawImage(source, 0, 0, w, h);
  const img = ctx.getImageData(0, 0, w, h);
  const hit = window.jsQR(img.data, w, h, { inversionAttempts: invert ? "attemptBoth" : "dontInvert" });
  return hit && hit.data ? hit.data : "";
}

async function loadImageSource(file) {
  // 优先 createImageBitmap；部分浏览器（或特殊格式）失败时退回 <img> 解码
  if (window.createImageBitmap) {
    try {
      const bitmap = await createImageBitmap(file);
      return { source: bitmap, w: bitmap.width, h: bitmap.height, release: () => bitmap.close && bitmap.close() };
    } catch (_) {}
  }
  const objectUrl = URL.createObjectURL(file);
  try {
    const img = new Image();
    await new Promise((res, rej) => {
      img.onload = res;
      img.onerror = rej;
      img.src = objectUrl;
    });
    return { source: img, w: img.naturalWidth, h: img.naturalHeight, release: () => URL.revokeObjectURL(objectUrl) };
  } catch (_) {
    URL.revokeObjectURL(objectUrl);
    throw new Error("无法读取这张图片，请换一张试试");
  }
}

async function decodeQRFromFile(file) {
  await loadJsQR();
  const img = await loadImageSource(file);
  try {
    for (const edge of [1600, 1000, 640]) {
      const text = decodeFromCanvasSource(img.source, img.w, img.h, edge, true);
      if (text) return text;
    }
    return "";
  } finally {
    img.release();
  }
}

async function handleScanFile(file) {
  if (!file) return;
  scanSetStatus("正在识别…");
  try {
    const text = await decodeQRFromFile(file);
    if (!text) {
      scanSetStatus("没有识别到二维码。请让二维码占满画面、保持清晰后重试。", true);
      return;
    }
    const parsed = parseScannedPairing(text);
    if (parsed.error) {
      scanSetStatus(parsed.error, true);
      return;
    }
    finishPairingScan(parsed.code);
  } catch (err) {
    scanSetStatus(err.message || "识别失败，请重试", true);
  }
}

async function detectFrame(video, canvas, ctx) {
  const vw = video.videoWidth;
  const vh = video.videoHeight;
  if (!vw || !vh) return "";
  if (PAIR_QR.detector) {
    try {
      const found = await PAIR_QR.detector.detect(video);
      if (found && found.length && found[0].rawValue) return found[0].rawValue;
    } catch (_) {
      PAIR_QR.detector = null; // 本机的检测器不可用，改走 jsQR
    }
    if (PAIR_QR.detector) return "";
  }
  const scale = Math.min(1, 720 / Math.max(vw, vh));
  const w = Math.round(vw * scale);
  const h = Math.round(vh * scale);
  if (canvas.width !== w) canvas.width = w;
  if (canvas.height !== h) canvas.height = h;
  ctx.drawImage(video, 0, 0, w, h);
  const img = ctx.getImageData(0, 0, w, h);
  const hit = window.jsQR(img.data, w, h, { inversionAttempts: "dontInvert" });
  return hit && hit.data ? hit.data : "";
}

async function startLiveCamera() {
  const video = document.getElementById("scan-video");
  if (!video) return false;
  scanSetStatus("正在打开摄像头…");
  let stream;
  try {
    stream = await navigator.mediaDevices.getUserMedia({
      audio: false,
      video: { facingMode: { ideal: "environment" }, width: { ideal: 1280 }, height: { ideal: 720 } },
    });
  } catch (err) {
    const denied = err && (err.name === "NotAllowedError" || err.name === "SecurityError");
    scanSetStatus(denied ? "未授予摄像头权限，可以改用拍照识别。" : "无法打开摄像头，可以改用拍照识别。", true);
    return false;
  }
  PAIR_QR.stream = stream;
  PAIR_QR.active = true;
  video.srcObject = stream;
  try {
    await video.play();
  } catch (_) {}
  try {
    await loadJsQR();
  } catch (err) {
    stopLiveCamera();
    scanSetStatus(err.message, true);
    return false;
  }
  if ("BarcodeDetector" in window) {
    try {
      PAIR_QR.detector = new window.BarcodeDetector({ formats: ["qr_code"] });
    } catch (_) {
      PAIR_QR.detector = null;
    }
  }
  scanSetStatus("将二维码放入框内");

  const canvas = document.createElement("canvas");
  const ctx = canvas.getContext("2d", { willReadFrequently: true });
  const tick = async () => {
    if (!PAIR_QR.active) return;
    if (!PAIR_QR.decoding) {
      PAIR_QR.decoding = true;
      try {
        const text = await detectFrame(video, canvas, ctx);
        if (text && PAIR_QR.active) {
          const parsed = parseScannedPairing(text);
          if (parsed.error) {
            scanSetStatus(parsed.error, true);
          } else {
            finishPairingScan(parsed.code);
            return;
          }
        }
      } finally {
        PAIR_QR.decoding = false;
      }
    }
    if (PAIR_QR.active) PAIR_QR.timer = setTimeout(tick, 120);
  };
  PAIR_QR.timer = setTimeout(tick, 120);
  return true;
}

async function startPairingScan() {
  const overlay = document.getElementById("scan-overlay");
  if (!overlay) return;
  overlay.classList.remove("hidden");
  const live = canUseLiveCamera();
  overlay.classList.toggle("no-live", !live);
  scanShowFallbackActions(true);
  if (!live) {
    scanSetStatus(
      window.isSecureContext
        ? "当前浏览器不支持实时摄像头，请拍照识别。"
        : "当前是 http 访问，浏览器不允许实时摄像头。请拍下配对二维码，或从相册选择。",
    );
    loadJsQR().catch(() => {});
    return;
  }
  const ok = await startLiveCamera();
  overlay.classList.toggle("no-live", !ok);
}

function initPairingScan() {
  document.getElementById("btn-pairing-scan")?.addEventListener("click", startPairingScan);
  document.getElementById("btn-scan-cancel")?.addEventListener("click", closePairingScan);
  document.getElementById("btn-scan-photo")?.addEventListener("click", () => document.getElementById("scan-file-camera")?.click());
  document.getElementById("btn-scan-gallery")?.addEventListener("click", () => document.getElementById("scan-file-gallery")?.click());
  for (const id of ["scan-file-camera", "scan-file-gallery"]) {
    const input = document.getElementById(id);
    input?.addEventListener("change", () => {
      const file = input.files && input.files[0];
      input.value = "";
      handleScanFile(file);
    });
  }
  document.addEventListener("visibilitychange", () => {
    if (document.hidden && PAIR_QR.active) closePairingScan();
  });
  window.addEventListener("pagehide", stopLiveCamera);
}
