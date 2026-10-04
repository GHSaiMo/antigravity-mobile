package main

import (
	"net/http"
	"strconv"
)

// desktopPairingHTML is served instead of the desktop workbench when LAN/public auth is enabled
// and the browser has not been paired yet. The workbench itself has no pairing UI and all of its
// protected assets and RPCs answer 401, which used to leave an empty page.
const desktopPairingHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Multigravity - 设备配对</title>
<link rel="icon" type="image/x-icon" href="/favicon.ico?v=3" />
<style>
  :root { color-scheme: light dark; --bg:#f5f5f7; --card:#fff; --fg:#1d1d1f; --sub:#6e6e73; --line:#d2d2d7; --accent:#0a84ff; --err:#d70015; }
  @media (prefers-color-scheme: dark) { :root { --bg:#131313; --card:#1e1e20; --fg:#f5f5f7; --sub:#98989f; --line:#38383a; --err:#ff6961; } }
  * { box-sizing: border-box; }
  body { margin:0; min-height:100vh; display:flex; align-items:center; justify-content:center; background:var(--bg); color:var(--fg);
         font-family:-apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif; }
  .card { width:min(440px,92vw); background:var(--card); border:1px solid var(--line); border-radius:16px; padding:28px; }
  h1 { font-size:20px; margin:0 0 6px; }
  p { margin:0 0 18px; font-size:13.5px; line-height:1.55; color:var(--sub); }
  input { width:100%; padding:11px 12px; font-size:15px; border:1px solid var(--line); border-radius:10px; background:transparent; color:var(--fg); font-family:ui-monospace,Menlo,monospace; }
  input:focus { outline:2px solid var(--accent); outline-offset:-1px; }
  button { width:100%; margin-top:14px; padding:11px; font-size:15px; font-weight:600; border:0; border-radius:10px; background:var(--accent); color:#fff; cursor:pointer; }
  button:disabled { opacity:.55; cursor:default; }
  #err { margin-top:12px; font-size:13px; color:var(--err); min-height:1em; }
  .hint { margin-top:16px; font-size:12px; color:var(--sub); }
  code { font-family:ui-monospace,Menlo,monospace; }
</style>
</head>
<body>
<form class="card" id="f" autocomplete="off">
  <h1>需要配对此设备</h1>
  <p>网关已开启局域网 / 公网访问验证。请输入配对码或配对链接（<code>agy://pair?...&amp;code=...</code>）后继续使用桌面工作台。</p>
  <input id="code" placeholder="配对码 / 配对链接" autofocus spellcheck="false">
  <button id="btn" type="submit">配对并进入</button>
  <div id="err" role="alert"></div>
  <div class="hint">配对码可在运行网关的电脑终端查看（二维码下方），或在已配对的手机客户端中生成。</div>
</form>
<script>
(function () {
  var input = document.getElementById("code"), err = document.getElementById("err"), btn = document.getElementById("btn");
  var qs = new URLSearchParams(location.search);
  var pre = qs.get("pair_code") || qs.get("code");
  if (pre) { input.value = pre; history.replaceState({}, "", location.pathname + location.hash); }
  function parse(raw) {
    raw = (raw || "").trim();
    if (raw.indexOf("agy://") === 0) {
      var m = raw.match(/[?&]code=([A-Za-z0-9_-]+)/);
      if (m) return m[1];
    }
    return raw;
  }
  document.getElementById("f").addEventListener("submit", function (e) {
    e.preventDefault();
    var code = parse(input.value);
    if (!code) { err.textContent = "请输入有效的配对码或配对链接"; return; }
    btn.disabled = true; err.textContent = "";
    fetch("/api/v1/auth/pair", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ pairing_code: code, device_name: "Web Browser (Desktop)", platform: "web" })
    }).then(function (r) {
      return r.json().catch(function () { return {}; }).then(function (d) {
        if (!r.ok) throw new Error(d.error || ("配对失败 (HTTP " + r.status + ")"));
        try { localStorage.setItem("agy_device_id", d.device_id || ""); } catch (_) {}
        location.replace("/");
      });
    }).catch(function (ex) {
      err.textContent = ex.message || "配对失败，请检查配对码是否过期";
      btn.disabled = false;
    });
  });
})();
</script>
</body>
</html>`

func serveDesktopPairingPage(w http.ResponseWriter) {
	body := []byte(desktopPairingHTML)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
