package main

import (
	"fmt"
	"net/http"
	"strconv"

	"antigravity-mobile/internal/i18n"
	"antigravity-mobile/internal/proxy"
)

type desktopPairingCopy struct {
	htmlLang       string
	pageTitle      string
	heading        string
	desc           string
	placeholder    string
	submit         string
	hint           string
	errEmpty       string
	errHttpPrefix  string
	errExpired     string
}

func desktopPairingCopyFor(lang string) desktopPairingCopy {
	switch i18n.NormalizeLang(lang) {
	case i18n.LangZh:
		return desktopPairingCopy{
			htmlLang:      "zh-CN",
			pageTitle:     "Multigravity - 设备配对",
			heading:       "需要配对此设备",
			desc:          `网关已开启局域网 / 公网访问验证。请输入配对码或配对链接（<code>agy://pair?...&amp;code=...</code>）后继续使用桌面工作台。`,
			placeholder:   "配对码 / 配对链接",
			submit:        "配对并进入",
			hint:          "配对码可在运行网关的电脑终端查看（二维码下方），或在已配对的手机客户端中生成。",
			errEmpty:      "请输入有效的配对码或配对链接",
			errHttpPrefix: "配对失败 (HTTP ",
			errExpired:    "配对失败，请检查配对码是否过期",
		}
	case i18n.LangJa:
		return desktopPairingCopy{
			htmlLang:      "ja",
			pageTitle:     "Multigravity - デバイスのペアリング",
			heading:       "このデバイスをペアリング",
			desc:          `ゲートウェイで LAN / リモート認証が有効です。ペアリングコードまたはリンク（<code>agy://pair?...&amp;code=...</code>）を入力して続行してください。`,
			placeholder:   "ペアリングコード / リンク",
			submit:        "ペアリングして開始",
			hint:          "ペアリングコードはゲートウェイを起動した PC のターミナル（QR コード下）、またはペアリング済みスマホアプリから確認できます。",
			errEmpty:      "有効なペアリングコードまたはリンクを入力してください",
			errHttpPrefix: "ペアリングに失敗しました (HTTP ",
			errExpired:    "ペアリングに失敗しました。コードの有効期限を確認してください。",
		}
	default:
		return desktopPairingCopy{
			htmlLang:      "en",
			pageTitle:     "Multigravity - Device Pairing",
			heading:       "Pair This Device",
			desc:          `Gateway LAN / remote authentication is enabled. Enter a pairing code or link (<code>agy://pair?...&amp;code=...</code>) to continue to the desktop workbench.`,
			placeholder:   "Pairing code / link",
			submit:        "Pair & Continue",
			hint:          "Find the pairing code in the terminal running mgy (below the QR code) or generate one from a paired mobile app.",
			errEmpty:      "Please enter a valid pairing code or link",
			errHttpPrefix: "Pairing failed (HTTP ",
			errExpired:    "Pairing failed. Check whether the code has expired.",
		}
	}
}

func renderDesktopPairingHTML(lang string) string {
	c := desktopPairingCopyFor(lang)
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="%s">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<link rel="icon" type="image/x-icon" href="/favicon.ico?v=4" />
<style>
  :root { color-scheme: light dark; --bg:#f5f5f7; --card:#fff; --fg:#1d1d1f; --sub:#6e6e73; --line:#d2d2d7; --accent:#0a84ff; --err:#d70015; }
  @media (prefers-color-scheme: dark) { :root { --bg:#131313; --card:#1e1e20; --fg:#f5f5f7; --sub:#98989f; --line:#38383a; --err:#ff6961; } }
  * { box-sizing: border-box; }
  body { margin:0; min-height:100vh; display:flex; align-items:center; justify-content:center; background:var(--bg); color:var(--fg);
         font-family:-apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Hiragino Sans","Microsoft YaHei",sans-serif; }
  .card { width:min(440px,92vw); background:var(--card); border:1px solid var(--line); border-radius:16px; padding:28px; position:relative; }
  .lang-bar { display:flex; justify-content:flex-end; gap:8px; margin:-8px 0 12px; font-size:12px; }
  .lang-bar button { width:auto; margin:0; padding:3px 8px; font-size:12px; font-weight:500; border-radius:6px; background:transparent; color:var(--sub); border:1px solid transparent; cursor:pointer; }
  .lang-bar button.active { color:var(--fg); border-color:var(--line); }
  h1 { font-size:20px; margin:0 0 6px; }
  p { margin:0 0 18px; font-size:13.5px; line-height:1.55; color:var(--sub); }
  input { width:100%%; padding:11px 12px; font-size:15px; border:1px solid var(--line); border-radius:10px; background:transparent; color:var(--fg); font-family:ui-monospace,Menlo,monospace; }
  input:focus { outline:2px solid var(--accent); outline-offset:-1px; }
  button.primary { width:100%%; margin-top:14px; padding:11px; font-size:15px; font-weight:600; border:0; border-radius:10px; background:var(--accent); color:#fff; cursor:pointer; }
  button.primary:disabled { opacity:.55; cursor:default; }
  #err { margin-top:12px; font-size:13px; color:var(--err); min-height:1em; }
  .hint { margin-top:16px; font-size:12px; color:var(--sub); }
  code { font-family:ui-monospace,Menlo,monospace; }
</style>
</head>
<body>
<form class="card" id="f" autocomplete="off">
  <div class="lang-bar" aria-label="Language">
    <button type="button" data-lang="en">English</button>
    <button type="button" data-lang="ja">日本語</button>
    <button type="button" data-lang="zh">简体中文</button>
  </div>
  <h1>%s</h1>
  <p>%s</p>
  <input id="code" placeholder="%s" autofocus spellcheck="false">
  <button class="primary" id="btn" type="submit">%s</button>
  <div id="err" role="alert"></div>
  <div class="hint">%s</div>
</form>
<script>
(function () {
  var curLang = %q;
  document.querySelectorAll(".lang-bar button[data-lang]").forEach(function (b) {
    if (b.getAttribute("data-lang") === curLang) b.classList.add("active");
    b.addEventListener("click", function () {
      var l = b.getAttribute("data-lang");
      document.cookie = "mgy_lang=" + l + "; Path=/; Max-Age=31536000; SameSite=Lax";
      try { localStorage.setItem("antigravity_language", l === "zh" ? "zh-CN" : l); } catch (_) {}
      location.reload();
    });
  });
  var input = document.getElementById("code"), err = document.getElementById("err"), btn = document.getElementById("btn");
  var qs = new URLSearchParams(location.search);
  var pre = qs.get("pair_code") || qs.get("code");
  if (pre) { input.value = pre; history.replaceState({}, "", location.pathname + location.hash); }
  function deviceKey() {
    try {
      var k = localStorage.getItem("agy_device_key");
      if (!k) {
        k = (window.crypto && crypto.randomUUID) ? crypto.randomUUID() : (Date.now().toString(36) + Math.random().toString(36).slice(2) + Math.random().toString(36).slice(2));
        if (k.length < 16) k += "0000000000000000";
        localStorage.setItem("agy_device_key", k);
      }
      return k;
    } catch (_) { return ""; }
  }
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
    if (!code) { err.textContent = %q; return; }
    btn.disabled = true; err.textContent = "";
    fetch("/api/v1/auth/pair", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ pairing_code: code, device_name: "Web Browser (Desktop)", platform: "web", device_key: deviceKey() })
    }).then(function (r) {
      return r.json().catch(function () { return {}; }).then(function (d) {
        if (!r.ok) throw new Error(d.error || (%q + r.status + ")"));
        try { localStorage.setItem("agy_device_id", d.device_id || ""); } catch (_) {}
        location.replace("/");
      });
    }).catch(function (ex) {
      err.textContent = ex.message || %q;
      btn.disabled = false;
    });
  });
})();
</script>
</body>
</html>`, c.htmlLang, c.pageTitle, c.heading, c.desc, c.placeholder, c.submit, c.hint, i18n.NormalizeLang(lang), c.errEmpty, c.errHttpPrefix, c.errExpired)
}

func serveDesktopPairingPage(w http.ResponseWriter, r *http.Request) {
	lang := proxy.ResolveDesktopLang(r)
	body := []byte(renderDesktopPairingHTML(lang))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Vary", "Cookie, Accept-Language")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
