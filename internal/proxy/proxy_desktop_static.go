package proxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/i18n"
)

// IsDesktopStaticPath checks if a path belongs to upstream desktop workbench static assets.
func IsDesktopStaticPath(path string) bool {
	return path == "/main.js" ||
		path == "/jetbox.css" ||
		path == "/compiled_tailwind.css" ||
		path == "/prism_bundle.js" ||
		path == "/diff_worker.js" ||
		path == "/audio_processor.js" ||
		path == "/icon.png" ||
		strings.HasPrefix(path, "/symbols-icons/")
}

type desktopStaticCacheItem struct {
	contentType string
	etag        string
	rawBody     []byte
	gzipBody    []byte
}

const maxDesktopStaticCacheEntries = 256

type desktopStaticFlight struct {
	done   chan struct{}
	item   *desktopStaticCacheItem
	status int
	body   []byte
	err    error
}

var (
	desktopStaticCacheMu   sync.RWMutex
	desktopStaticCache     = make(map[string]*desktopStaticCacheItem)
	desktopRawMainJS       []byte
	desktopRawMainJSPort   int
	desktopStaticFlightsMu sync.Mutex
	desktopStaticFlights   = make(map[string]*desktopStaticFlight)

	desktopIndexCacheMu    sync.RWMutex
	desktopIndexCacheHTML  = make(map[string][]byte)
	desktopIndexCachePort  int
	desktopIndexCacheToken string

	reDesktopCSRF           = regexp.MustCompile(`"csrfToken":"[^"]*"`)
	reDesktopProduct        = regexp.MustCompile(`"productName":"[^"]*"`)
	reDesktopTitle          = regexp.MustCompile(`(?i)<title>[^<]*</title>`)
	reDesktopPreconnect     = regexp.MustCompile(`(?s)<link\s+rel="preconnect"\s+href="https?://(?:fonts\.|[a-z0-9-]+\.)?gstatic\.com[^"]*"\s*(?:crossorigin)?\s*/?>\s*`)
	reDesktopFontPreconnect = regexp.MustCompile(`(?s)<link\s+rel="preconnect"\s+href="https?://fonts\.googleapis\.com[^"]*"\s*(?:crossorigin)?\s*/?>\s*`)
	reDesktopFavicon        = regexp.MustCompile(`(?s)<link\s+(?:[^"'<>]|"[^"]*"|'[^']*')*rel=["'](?:shortcut\s+)?icon["'](?:[^"'<>]|"[^"]*"|'[^']*')*/?\s*>`)
)

func putDesktopStaticCache(key string, item *desktopStaticCacheItem) {
	desktopStaticCacheMu.Lock()
	if len(desktopStaticCache) >= maxDesktopStaticCacheEntries {
		for k := range desktopStaticCache {
			if !strings.HasPrefix(k, "/main.js") {
				delete(desktopStaticCache, k)
				break
			}
		}
	}
	desktopStaticCache[key] = item
	desktopStaticCacheMu.Unlock()
}

// ClearDesktopStaticCache clears cached static assets when the upstream instance changes.
func ClearDesktopStaticCache() {
	desktopStaticCacheMu.Lock()
	desktopStaticCache = make(map[string]*desktopStaticCacheItem)
	desktopRawMainJS = nil
	desktopRawMainJSPort = 0
	desktopStaticCacheMu.Unlock()

	desktopIndexCacheMu.Lock()
	desktopIndexCacheHTML = make(map[string][]byte)
	desktopIndexCachePort = 0
	desktopIndexCacheToken = ""
	desktopIndexCacheMu.Unlock()
}

func buildMainJSVariant(rawUpstream []byte, lang string) *desktopStaticCacheItem {
	localized := localizeMainJSCheckedForLang(rawUpstream, lang)
	h := sha256.Sum256(localized)
	etag := fmt.Sprintf(`W/"%x-%s"`, h[:8], lang)

	var gzBuf bytes.Buffer
	gw, _ := gzip.NewWriterLevel(&gzBuf, gzip.BestSpeed)
	_, _ = gw.Write(localized)
	_ = gw.Close()

	return &desktopStaticCacheItem{
		contentType: "application/javascript; charset=utf-8",
		etag:        etag,
		rawBody:     localized,
		gzipBody:    gzBuf.Bytes(),
	}
}

// WarmupDesktopStatic proactively caches upstream main.js in memory and pre-builds the
// active gateway language variant so desktop page loads have near-zero latency.
func (p *Proxy) WarmupDesktopStatic(port int, token string) {
	if port == 0 || p.mediumClient == nil {
		return
	}
	targetURL := fmt.Sprintf("https://127.0.0.1:%d/main.js", port)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return
	}
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}
	resp, err := p.mediumClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return
	}

	rawBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	desktopStaticCacheMu.Lock()
	desktopRawMainJS = rawBytes
	desktopRawMainJSPort = port
	desktopStaticCacheMu.Unlock()

	defaultLang := i18n.GetLang()
	item := buildMainJSVariant(rawBytes, defaultLang)
	putDesktopStaticCache("/main.js:"+defaultLang, item)
	slog.Info(fmt.Sprintf("[Proxy] ⚡ Pre-warmed desktop main.js [%s] (%d KB gzip)", defaultLang, len(item.gzipBody)/1024))

	// Proactively pre-warm GetAuthStatus and GetCascadeNuxes in background so initial desktop load has 0ms latency
	go func() {
		// 1. Pre-warm GetAuthStatus (upstream Google Auth latency is 3.5s - 6.5s)
		authURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetAuthStatus", port)
		authReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, authURL, strings.NewReader("{}"))
		if err == nil {
			authReq.Header.Set("Content-Type", "application/json")
			authReq.Header.Set("Connect-Protocol-Version", "1")
			if token != "" {
				authReq.Header.Set("x-codeium-csrf-token", token)
			}
			resp, err := p.mediumClient.Do(authReq)
			if err == nil {
				if resp.StatusCode == http.StatusOK {
					if data, err := io.ReadAll(resp.Body); err == nil && len(data) > 0 {
						p.authStatusCacheMu.Lock()
						p.authStatusCacheBody = data
						p.authStatusCacheHeaders = resp.Header.Clone()
						p.authStatusCachedAt = time.Now()
						p.authStatusCacheMu.Unlock()
						slog.Info("[Proxy] ⚡ Pre-warmed GetAuthStatus cache (0ms first load)")
					}
				} else {
					_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
				}
				_ = resp.Body.Close()
			}
		}

		// 2. Pre-warm GetCascadeNuxes (upstream latency 300-900ms)
		nuxClient := p.shortClient
		if nuxClient == nil {
			nuxClient = p.mediumClient
		}
		if nuxClient == nil {
			return
		}
		nuxURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetCascadeNuxes", port)
		nuxReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, nuxURL, strings.NewReader("{}"))
		if err == nil {
			nuxReq.Header.Set("Content-Type", "application/json")
			nuxReq.Header.Set("Connect-Protocol-Version", "1")
			if token != "" {
				nuxReq.Header.Set("x-codeium-csrf-token", token)
			}
			resp, err := nuxClient.Do(nuxReq)
			if err == nil {
				if resp.StatusCode == http.StatusOK {
					if data, err := io.ReadAll(resp.Body); err == nil && len(data) > 0 {
						p.nuxCacheMu.Lock()
						p.nuxCacheBody = data
						p.nuxCacheHeaders = resp.Header.Clone()
						p.nuxCachedAt = time.Now()
						p.nuxCacheMu.Unlock()
						slog.Info("[Proxy] ⚡ Pre-warmed GetCascadeNuxes cache (0ms first load)")
					}
				} else {
					_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
				}
				_ = resp.Body.Close()
			}
		}
	}()
}

// desktopStaticCacheControl lets browsers keep static assets for a day, except main.js: its
// content changes whenever the localization dictionaries or selected language change.
func desktopStaticCacheControl(path string) string {
	if path == "/main.js" {
		return "no-cache"
	}
	return "public, max-age=86400, stale-while-revalidate=604800"
}

// HandleDesktopStatic serves desktop static assets with in-memory caching, pre-compressed gzip,
// per-language desktop bundle localization (zh / en / ja), and conditional 304 validation.
func (p *Proxy) HandleDesktopStatic(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	port := p.activePort
	token := p.activeToken
	p.mu.RUnlock()

	if port == 0 {
		http.Error(w, "Antigravity language_server is not connected", http.StatusServiceUnavailable)
		return
	}

	path := r.URL.Path
	lang := ""
	cacheKey := path
	if path == "/main.js" {
		lang = ResolveDesktopLang(r)
		cacheKey = "/main.js:" + lang
	}

	desktopStaticCacheMu.RLock()
	item := desktopStaticCache[cacheKey]
	cachedRawMain := desktopRawMainJS
	cachedRawPort := desktopRawMainJSPort
	desktopStaticCacheMu.RUnlock()

	if item == nil {
		flightKey := fmt.Sprintf("%d:%s", port, cacheKey)
		desktopStaticFlightsMu.Lock()
		fl, inFlight := desktopStaticFlights[flightKey]
		if !inFlight {
			fl = &desktopStaticFlight{done: make(chan struct{})}
			desktopStaticFlights[flightKey] = fl
			desktopStaticFlightsMu.Unlock()

			func() {
				defer func() {
					desktopStaticFlightsMu.Lock()
					delete(desktopStaticFlights, flightKey)
					desktopStaticFlightsMu.Unlock()
					close(fl.done)
				}()

				// Fast path for /main.js: if raw upstream bundle is already cached in memory for this port,
				// derive the requested language variant locally without re-fetching from upstream.
				if path == "/main.js" && cachedRawPort == port && len(cachedRawMain) > 0 {
					built := buildMainJSVariant(cachedRawMain, lang)
					putDesktopStaticCache(cacheKey, built)
					fl.item = built
					fl.status = http.StatusOK
					return
				}

				targetURL := fmt.Sprintf("https://127.0.0.1:%d%s", port, path)
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
				if err != nil {
					fl.err = err
					fl.status = http.StatusInternalServerError
					return
				}
				if token != "" {
					req.Header.Set("x-codeium-csrf-token", token)
				}
				resp, err := p.mediumClient.Do(req)
				if err != nil {
					fl.err = err
					fl.status = http.StatusBadGateway
					return
				}
				defer resp.Body.Close()

				rawBytes, readErr := io.ReadAll(resp.Body)
				if resp.StatusCode != http.StatusOK {
					fl.status = resp.StatusCode
					fl.body = rawBytes
					return
				}
				if readErr != nil {
					fl.err = readErr
					fl.status = http.StatusInternalServerError
					return
				}

				if path == "/main.js" {
					desktopStaticCacheMu.Lock()
					desktopRawMainJS = rawBytes
					desktopRawMainJSPort = port
					desktopStaticCacheMu.Unlock()

					built := buildMainJSVariant(rawBytes, lang)
					putDesktopStaticCache(cacheKey, built)
					fl.item = built
					fl.status = http.StatusOK
					return
				}

				cType := resp.Header.Get("Content-Type")
				if cType == "" {
					cType = "application/javascript; charset=utf-8"
				}
				etag := resp.Header.Get("Etag")
				if etag == "" {
					h := sha256.Sum256(rawBytes)
					etag = fmt.Sprintf(`W/"%x"`, h[:8])
				}

				var gzBuf bytes.Buffer
				gw, _ := gzip.NewWriterLevel(&gzBuf, gzip.BestSpeed)
				_, _ = gw.Write(rawBytes)
				_ = gw.Close()

				built := &desktopStaticCacheItem{
					contentType: cType,
					etag:        etag,
					rawBody:     rawBytes,
					gzipBody:    gzBuf.Bytes(),
				}
				putDesktopStaticCache(cacheKey, built)
				fl.item = built
				fl.status = http.StatusOK
			}()
		} else {
			desktopStaticFlightsMu.Unlock()
			select {
			case <-fl.done:
			case <-r.Context().Done():
				return
			}
		}

		if fl.err != nil {
			http.Error(w, fl.err.Error(), fl.status)
			return
		}
		if fl.item == nil {
			status := fl.status
			if status == 0 {
				status = http.StatusBadGateway
			}
			w.WriteHeader(status)
			_, _ = w.Write(fl.body)
			return
		}
		item = fl.item
	}

	// 304 Not Modified validation
	if match := r.Header.Get("If-None-Match"); match != "" && (match == item.etag || match == "*") {
		w.Header().Set("ETag", item.etag)
		w.Header().Set("Cache-Control", desktopStaticCacheControl(path))
		if path == "/main.js" {
			w.Header().Set("Vary", "Accept-Encoding, Cookie, Accept-Language")
		}
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", item.contentType)
	w.Header().Set("ETag", item.etag)
	w.Header().Set("Cache-Control", desktopStaticCacheControl(path))
	if path == "/main.js" {
		w.Header().Set("Vary", "Accept-Encoding, Cookie, Accept-Language")
	} else {
		w.Header().Set("Vary", "Accept-Encoding")
	}

	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") && len(item.gzipBody) > 0 {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Length", strconv.Itoa(len(item.gzipBody)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(item.gzipBody)
	} else {
		w.Header().Set("Content-Length", strconv.Itoa(len(item.rawBody)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(item.rawBody)
	}
}

func desktopConnectingSplashHTML(lang string) []byte {
	htmlLang := "en"
	title := "Starting Multigravity..."
	heading := "Connecting to Multigravity Agent Service..."
	sub := "The workbench will load automatically once language_server starts"
	switch i18n.NormalizeLang(lang) {
	case i18n.LangZh:
		htmlLang = "zh-CN"
		title = "Multigravity 启动中..."
		heading = "正在连接 Multigravity 智能体服务..."
		sub = "language_server 启动后将自动载入工作台"
	case i18n.LangJa:
		htmlLang = "ja"
		title = "Multigravity を起動中..."
		heading = "Multigravity エージェントサービスに接続中..."
		sub = "language_server の起動後にワークベンチを自動で読み込みます"
	}
	return []byte(fmt.Sprintf(`<!DOCTYPE html>
<html lang="%s">
<head>
  <meta charset="UTF-8">
  <title>%s</title>
  <link rel="icon" type="image/x-icon" href="/favicon.ico?v=4" />
  <link rel="icon" type="image/png" sizes="32x32" href="/icons/favicon-32.png?v=4" />
  <link rel="icon" type="image/png" sizes="192x192" href="/icons/icon-192.png?v=4" />
  <link rel="apple-touch-icon" href="/icons/apple-touch-icon.png?v=4" />
  <style>
    body { background: #131313; color: #e2e8f0; font-family: -apple-system, BlinkMacSystemFont, sans-serif; display: flex; flex-direction: column; align-items: center; justify-content: center; height: 100vh; margin: 0; }
    .spinner { width: 36px; height: 36px; border: 3px solid rgba(255,255,255,0.1); border-top-color: #38bdf8; border-radius: 50%%; animation: spin 0.8s linear infinite; margin-bottom: 16px; }
    @keyframes spin { to { transform: rotate(360deg); } }
  </style>
</head>
<body>
  <div class="spinner"></div>
  <h2>%s</h2>
  <p style="color: #94a3b8; font-size: 14px;">%s</p>
  <script>setTimeout(() => location.reload(), 2000);</script>
</body>
</html>`, htmlLang, title, heading, sub))
}

func desktopTextInterceptorJS(lang string) string {
	switch i18n.NormalizeLang(lang) {
	case i18n.LangZh:
		return `      // High-performance O(1) text node and attribute interceptor for dynamic Chinese strings
      (() => {
        const dict = {
          "Ask anything, @ to mention, / for actions": "输入任意内容，@ 提及，/ 触发指令",
          "Ask anything, @ to mention": "输入任意内容，@ 提及",
          ", / for actions": "，/ 触发指令",
          "No Model Selected": "未选择模型",
          "No Models Available": "无可用模型",
          "No models are available for your current project or organization.": "当前项目或组织暂无可用模型。",
          "No Project": "无项目",
          "Main Agent": "主智能体",
          "Conversations": "所有会话",
          "Workspaces": "工作区",
          "New Conversation": "新建会话",
          "New Workspace": "新建工作区",
          "Thinking...": "思考中...",
          "No conversations yet": "暂无会话",
          "Pin": "置顶",
          "Unpin": "取消置顶",
          "Delete": "删除",
          "Rename": "重命名"
        };
        const origCreateText = document.createTextNode.bind(document);
        document.createTextNode = function(text) {
          if (typeof text === 'string') {
            const tr = text.trim();
            if (dict[tr]) {
              text = text.replace(tr, dict[tr]);
            }
          }
          return origCreateText(text);
        };
        const origSetAttr = Element.prototype.setAttribute;
        Element.prototype.setAttribute = function(name, val) {
          if (typeof val === 'string' && (name === 'placeholder' || name === 'title' || name === 'aria-label')) {
            const tr = val.trim();
            if (dict[tr]) {
              val = val.replace(tr, dict[tr]);
            }
          }
          return origSetAttr.call(this, name, val);
        };
      })();`
	case i18n.LangJa:
		return `      // High-performance O(1) text node and attribute interceptor for dynamic Japanese strings
      (() => {
        const dict = {
          "Ask anything, @ to mention, / for actions": "何でも質問、@ でメンション、/ でコマンド",
          "Ask anything, @ to mention": "何でも質問、@ でメンション",
          ", / for actions": "、/ でコマンド",
          "No Model Selected": "モデル未選択",
          "No Models Available": "利用可能なモデルがありません",
          "No models are available for your current project or organization.": "現在のプロジェクトまたは組織で利用可能なモデルがありません。",
          "No Project": "プロジェクトなし",
          "Main Agent": "メインエージェント",
          "Conversations": "会話一覧",
          "Workspaces": "ワークスペース",
          "New Conversation": "新しい会話",
          "New Workspace": "新規ワークスペース",
          "Thinking...": "思考中...",
          "No conversations yet": "会話はまだありません",
          "Pin": "ピン留め",
          "Unpin": "ピン留め解除",
          "Delete": "削除",
          "Rename": "名前を変更"
        };
        const origCreateText = document.createTextNode.bind(document);
        document.createTextNode = function(text) {
          if (typeof text === 'string') {
            const tr = text.trim();
            if (dict[tr]) {
              text = text.replace(tr, dict[tr]);
            }
          }
          return origCreateText(text);
        };
        const origSetAttr = Element.prototype.setAttribute;
        Element.prototype.setAttribute = function(name, val) {
          if (typeof val === 'string' && (name === 'placeholder' || name === 'title' || name === 'aria-label')) {
            const tr = val.trim();
            if (dict[tr]) {
              val = val.replace(tr, dict[tr]);
            }
          }
          return origSetAttr.call(this, name, val);
        };
      })();`
	default:
		return ""
	}
}

func desktopLangSwitcherJS(lang, pref string) string {
	autoLabel := "Auto"
	ariaLabel := "Language"
	switch i18n.NormalizeLang(lang) {
	case i18n.LangZh:
		autoLabel = "自动"
		ariaLabel = "界面语言"
	case i18n.LangJa:
		autoLabel = "自動"
		ariaLabel = "表示言語"
	}
	return fmt.Sprintf(`      // Subtle native-matching Desktop Workbench language selector (sidebar footer next to Settings)
      (() => {
        const currentPref = %q;
        const autoLabel = %q;
        const ariaLabel = %q;
        const settingsRe = /^(Settings|系统设置|設定)$/;
        function mountLangControl() {
          if (document.getElementById("mgy-desktop-lang-select")) return true;
          const btns = document.querySelectorAll(".bg-sidebar button");
          let settingsBtn = null;
          for (let i = btns.length - 1; i >= 0; i--) {
            const txt = (btns[i].textContent || "").trim();
            if (settingsRe.test(txt)) {
              settingsBtn = btns[i];
              break;
            }
          }
          if (!settingsBtn || !settingsBtn.parentElement) return false;
          const parent = settingsBtn.parentElement;
          const row = document.createElement("div");
          row.id = "mgy-desktop-footer-row";
          row.style.cssText = "display:flex;align-items:center;gap:4px;width:100%%;";
          parent.insertBefore(row, settingsBtn);
          row.appendChild(settingsBtn);
          settingsBtn.style.flex = "1 1 auto";
          settingsBtn.style.minWidth = "0";
          settingsBtn.style.width = "auto";

          const sel = document.createElement("select");
          sel.id = "mgy-desktop-lang-select";
          sel.setAttribute("aria-label", ariaLabel);
          sel.title = ariaLabel;
          sel.style.cssText = "flex:0 0 auto;background:transparent;color:inherit;opacity:0.72;border:none;border-radius:6px;padding:4px 4px;font-size:12px;cursor:pointer;outline:none;max-width:78px;";
          const opts = [
            ["auto", autoLabel],
            ["en", "English"],
            ["ja", "日本語"],
            ["zh", "简体中文"]
          ];
          opts.forEach(([v, l]) => {
            const o = document.createElement("option");
            o.value = v;
            o.textContent = l;
            sel.appendChild(o);
          });
          sel.value = currentPref;
          sel.addEventListener("mouseenter", () => { sel.style.opacity = "1"; });
          sel.addEventListener("mouseleave", () => { sel.style.opacity = "0.72"; });
          sel.addEventListener("change", () => {
            const v = sel.value || "auto";
            if (v === "auto") {
              document.cookie = "mgy_lang=; Path=/; Max-Age=0; SameSite=Lax";
              try { localStorage.removeItem("antigravity_language"); } catch (_) {}
            } else {
              document.cookie = "mgy_lang=" + v + "; Path=/; Max-Age=31536000; SameSite=Lax";
              try { localStorage.setItem("antigravity_language", v === "zh" ? "zh-CN" : v); } catch (_) {}
            }
            const u = new URL(location.href);
            u.searchParams.delete("lang");
            location.replace(u.toString());
          });
          row.appendChild(sel);
          return true;
        }
        function start() {
          if (mountLangControl()) return;
          const obs = new MutationObserver(() => {
            if (mountLangControl()) obs.disconnect();
          });
          obs.observe(document.documentElement, { childList: true, subtree: true });
        }
        if (document.readyState === "loading") {
          document.addEventListener("DOMContentLoaded", start);
        } else {
          start();
        }
      })();`, pref, autoLabel, ariaLabel)
}

// HandleDesktopIndex serves the official desktop web index.html with live CSRF injection,
// per-language localization (zh / en / ja), fast memory caching, and tablet detection redirect.
func (p *Proxy) HandleDesktopIndex(w http.ResponseWriter, r *http.Request) {
	lang := ResolveDesktopLang(r)
	pref := ResolveDesktopLangPref(r)

	// Persist explicit ?lang= override in cookie so subsequent /main.js and RPC calls stay aligned.
	if q := strings.TrimSpace(r.URL.Query().Get("lang")); q != "" {
		isSecure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
		if pref == "auto" {
			http.SetCookie(w, &http.Cookie{
				Name:     DesktopLangCookieName,
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				SameSite: http.SameSiteLaxMode,
				Secure:   isSecure,
			})
		} else {
			http.SetCookie(w, &http.Cookie{
				Name:     DesktopLangCookieName,
				Value:    pref,
				Path:     "/",
				MaxAge:   86400 * 365,
				SameSite: http.SameSiteLaxMode,
				Secure:   isSecure,
			})
		}
	}

	p.mu.RLock()
	port := p.activePort
	token := p.activeToken
	p.mu.RUnlock()

	if port == 0 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Vary", "Cookie, Accept-Language")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(desktopConnectingSplashHTML(lang))
		return
	}

	effectiveToken := token
	if effectiveToken == "" {
		effectiveToken = "headless-csrf-token"
	}

	cacheKey := lang + "|" + pref

	// Fast path: serve cached transformed HTML if upstream port, token, lang, and pref are unchanged
	desktopIndexCacheMu.RLock()
	if desktopIndexCachePort == port && desktopIndexCacheToken == token {
		if cachedBytes, ok := desktopIndexCacheHTML[cacheKey]; ok && len(cachedBytes) > 0 {
			desktopIndexCacheMu.RUnlock()

			http.SetCookie(w, &http.Cookie{
				Name:     "csrfToken",
				Value:    effectiveToken,
				Path:     "/",
				SameSite: http.SameSiteLaxMode,
				Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
			})
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Vary", "Cookie, Accept-Language")
			w.Header().Set("Content-Length", strconv.Itoa(len(cachedBytes)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(cachedBytes)
			return
		}
	}
	desktopIndexCacheMu.RUnlock()

	targetURL := fmt.Sprintf("https://127.0.0.1:%d/", port)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, targetURL, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	resp, err := p.shortClient.Do(req)
	if err != nil {
		http.Error(w, "Upstream language_server error: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read upstream response: "+err.Error(), http.StatusInternalServerError)
		return
	}

	htmlStr := string(bodyBytes)

	// 1. Ensure fresh CSRF token is injected into window.__APP_CONFIG__
	htmlStr = reDesktopCSRF.ReplaceAllString(htmlStr, fmt.Sprintf(`"csrfToken":%q`, effectiveToken))

	// Set cookie so ConnectRPC client and WebSocket can read it
	http.SetCookie(w, &http.Cookie{
		Name:     "csrfToken",
		Value:    effectiveToken,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
	})

	// 2. Align productName to Multigravity in window.__APP_CONFIG__
	htmlStr = reDesktopProduct.ReplaceAllString(htmlStr, `"productName":"multigravity"`)

	// 3. Align page title
	htmlStr = reDesktopTitle.ReplaceAllString(htmlStr, "<title>Multigravity</title>")

	// 4. Set <html lang="..."> matching resolved Desktop Web Console language
	htmlLang := "en"
	manifestHref := "/manifest-en.json"
	switch lang {
	case i18n.LangZh:
		htmlLang = "zh-CN"
		manifestHref = "/manifest.json"
	case i18n.LangJa:
		htmlLang = "ja"
		manifestHref = "/manifest-ja.json"
	}
	htmlStr = strings.Replace(htmlStr, `<html lang="en">`, fmt.Sprintf(`<html lang="%s">`, htmlLang), 1)

	// 5. Remove blocking external preconnect links to Google font & gstatic CDNs
	htmlStr = reDesktopPreconnect.ReplaceAllString(htmlStr, "")
	htmlStr = reDesktopFontPreconnect.ReplaceAllString(htmlStr, "")

	// 6. Cache-busting + language-keyed query parameters on static assets
	htmlStr = strings.ReplaceAll(htmlStr, `src="/main.js"`, fmt.Sprintf(`src="/main.js?v=%s&lang=%s"`, desktopAssetVersion, lang))
	htmlStr = strings.ReplaceAll(htmlStr, `src="/prism_bundle.js"`, fmt.Sprintf(`src="/prism_bundle.js?v=%s"`, desktopAssetVersion))
	htmlStr = strings.ReplaceAll(htmlStr, `href="/jetbox.css"`, fmt.Sprintf(`href="/jetbox.css?v=%s"`, desktopAssetVersion))
	htmlStr = strings.ReplaceAll(htmlStr, `href="/compiled_tailwind.css"`, fmt.Sprintf(`href="/compiled_tailwind.css?v=%s"`, desktopAssetVersion))

	// 7. Icons, metadata, tablet detection, telemetry stub, language-specific interceptor, and subtle switcher
	multigravityIconsMeta := `    <link rel="icon" type="image/x-icon" href="/favicon.ico?v=4" />
    <link rel="icon" type="image/png" sizes="32x32" href="/icons/favicon-32.png?v=4" />
    <link rel="icon" type="image/png" sizes="192x192" href="/icons/icon-192.png?v=4" />
    <link rel="apple-touch-icon" href="/icons/apple-touch-icon.png?v=4" />
    <link rel="manifest" href="` + manifestHref + `" />
    <meta name="apple-mobile-web-app-title" content="Multigravity" />
    <meta name="apple-mobile-web-app-capable" content="yes" />
    <meta name="apple-mobile-web-app-status-bar-style" content="black-translucent" />
    <meta name="theme-color" content="#0f172a" />
    <script>
      // Seamless iPadOS tablet detection: if masquerading as Mac desktop UA with touch points, redirect to PWA
      if (navigator.maxTouchPoints > 1 && !/Windows|Linux/.test(navigator.userAgent) && !location.search.includes("view=")) {
        location.replace("/?view=pwa");
      }
      // Proactively prevent trapping on /onboarding?login=true
      if (location.pathname === "/onboarding") {
        const q = new URLSearchParams(location.search);
        const target = q.get("redirect") || "/";
        location.replace(target.startsWith("/onboarding") ? "/" : target);
      }
      (() => {
        const origPush = history.pushState.bind(history);
        const origReplace = history.replaceState.bind(history);
        history.pushState = function(state, unused, url) {
          if (typeof url === "string" && url.includes("/onboarding")) {
            try {
              const u = new URL(url, location.origin);
              const target = u.searchParams.get("redirect") || "/";
              return origPush(state, unused, target.startsWith("/onboarding") ? "/" : target);
            } catch (_) {
              return origPush(state, unused, "/");
            }
          }
          return origPush(state, unused, url);
        };
        history.replaceState = function(state, unused, url) {
          if (typeof url === "string" && url.includes("/onboarding")) {
            try {
              const u = new URL(url, location.origin);
              const target = u.searchParams.get("redirect") || "/";
              return origReplace(state, unused, target.startsWith("/onboarding") ? "/" : target);
            } catch (_) {
              return origReplace(state, unused, "/");
            }
          }
          return origReplace(state, unused, url);
        };
      })();
      // Telemetry & external font network fast-stub (eliminates 15-30s browser network queue hangs in restricted environments)
      (() => {
        const blocked = ['play.google.com', 'fonts.googleapis.com', 'fonts.gstatic.com'];
        const origFetch = window.fetch;
        window.fetch = function(input, init) {
          const url = typeof input === 'string' ? input : (input && input.url ? input.url : '');
          if (blocked.some(b => url.includes(b))) {
            return Promise.resolve(new Response('{}', { status: 200, headers: { 'Content-Type': 'application/json' } }));
          }
          return origFetch.apply(this, arguments);
        };
        if (navigator.sendBeacon) {
          const origBeacon = navigator.sendBeacon.bind(navigator);
          navigator.sendBeacon = function(url, data) {
            if (typeof url === 'string' && blocked.some(b => url.includes(b))) return true;
            return origBeacon(url, data);
          };
        }
      })();
` + desktopTextInterceptorJS(lang) + "\n" + desktopLangSwitcherJS(lang, pref) + `
    </script>
    <script>` + desktopThemePresetFixJS + desktopRPCMuxJS + `</script>`
	if reDesktopFavicon.MatchString(htmlStr) {
		htmlStr = reDesktopFavicon.ReplaceAllString(htmlStr, multigravityIconsMeta)
	} else {
		htmlStr = strings.Replace(htmlStr, "<head>", "<head>\n"+multigravityIconsMeta, 1)
	}

	htmlBytes := []byte(htmlStr)
	desktopIndexCacheMu.Lock()
	if desktopIndexCachePort != port || desktopIndexCacheToken != token {
		desktopIndexCacheHTML = make(map[string][]byte)
		desktopIndexCachePort = port
		desktopIndexCacheToken = token
	}
	desktopIndexCacheHTML[cacheKey] = htmlBytes
	desktopIndexCacheMu.Unlock()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Vary", "Cookie, Accept-Language")
	w.Header().Set("Content-Length", strconv.Itoa(len(htmlBytes)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(htmlBytes)
}
