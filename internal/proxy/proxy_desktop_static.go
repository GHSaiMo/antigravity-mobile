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

var (
	desktopStaticCacheMu sync.RWMutex
	desktopStaticCache   = make(map[string]*desktopStaticCacheItem)

	desktopIndexCacheMu    sync.RWMutex
	desktopIndexCacheHTML  []byte
	desktopIndexCachePort  int
	desktopIndexCacheToken string
)

// ClearDesktopStaticCache clears cached static assets when the upstream instance changes.
func ClearDesktopStaticCache() {
	desktopStaticCacheMu.Lock()
	desktopStaticCache = make(map[string]*desktopStaticCacheItem)
	desktopStaticCacheMu.Unlock()

	desktopIndexCacheMu.Lock()
	desktopIndexCacheHTML = nil
	desktopIndexCachePort = 0
	desktopIndexCacheToken = ""
	desktopIndexCacheMu.Unlock()
}

// WarmupDesktopStatic proactively caches and localizes main.js in memory
// so the user experiences instantaneous zero-latency page loads on desktop.
func (p *Proxy) WarmupDesktopStatic(port int, token string) {
	if port == 0 {
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
	resp, err := p.shortClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	rawBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	rawBytes = localizeMainJSChecked(rawBytes)
	h := sha256.Sum256(rawBytes)
	etag := fmt.Sprintf(`W/"%x"`, h[:8])

	var gzBuf bytes.Buffer
	gw, _ := gzip.NewWriterLevel(&gzBuf, gzip.BestSpeed)
	_, _ = gw.Write(rawBytes)
	_ = gw.Close()

	item := &desktopStaticCacheItem{
		contentType: "application/javascript; charset=utf-8",
		etag:        etag,
		rawBody:     rawBytes,
		gzipBody:    gzBuf.Bytes(),
	}

	desktopStaticCacheMu.Lock()
	desktopStaticCache["/main.js"] = item
	desktopStaticCacheMu.Unlock()
	slog.Info(fmt.Sprintf("[Proxy] ⚡ Pre-warmed localized desktop main.js (%d KB gzip)", gzBuf.Len()/1024))

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
			if err == nil && resp.StatusCode == http.StatusOK {
				defer resp.Body.Close()
				if data, err := io.ReadAll(resp.Body); err == nil && len(data) > 0 {
					p.authStatusCacheMu.Lock()
					p.authStatusCacheBody = data
					p.authStatusCacheHeaders = resp.Header.Clone()
					p.authStatusCachedAt = time.Now()
					p.authStatusCacheMu.Unlock()
					slog.Info("[Proxy] ⚡ Pre-warmed GetAuthStatus cache (0ms first load)")
				}
			}
		}

		// 2. Pre-warm GetCascadeNuxes (upstream latency 300-900ms)
		nuxURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetCascadeNuxes", port)
		nuxReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, nuxURL, strings.NewReader("{}"))
		if err == nil {
			nuxReq.Header.Set("Content-Type", "application/json")
			nuxReq.Header.Set("Connect-Protocol-Version", "1")
			if token != "" {
				nuxReq.Header.Set("x-codeium-csrf-token", token)
			}
			resp, err := p.shortClient.Do(nuxReq)
			if err == nil && resp.StatusCode == http.StatusOK {
				defer resp.Body.Close()
				if data, err := io.ReadAll(resp.Body); err == nil && len(data) > 0 {
					p.nuxCacheMu.Lock()
					p.nuxCacheBody = data
					p.nuxCacheHeaders = resp.Header.Clone()
					p.nuxCachedAt = time.Now()
					p.nuxCacheMu.Unlock()
					slog.Info("[Proxy] ⚡ Pre-warmed GetCascadeNuxes cache (0ms first load)")
				}
			}
		}
	}()
}

// HandleDesktopStatic serves desktop static assets with in-memory caching, pre-compressed gzip,
// gateway-level zero-runtime-overhead Chinese translation for main.js, and conditional 304 validation.
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

	desktopStaticCacheMu.RLock()
	item := desktopStaticCache[path]
	desktopStaticCacheMu.RUnlock()

	if item == nil {
		targetURL := fmt.Sprintf("https://127.0.0.1:%d%s", port, path)
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
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			w.WriteHeader(resp.StatusCode)
			_, _ = io.Copy(w, resp.Body)
			return
		}

		rawBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		cType := resp.Header.Get("Content-Type")
		if cType == "" {
			cType = "application/javascript; charset=utf-8"
		}

		// Gateway-level zero-runtime-overhead translation for main.js
		if path == "/main.js" {
			rawBytes = localizeMainJSChecked(rawBytes)
		}

		etag := resp.Header.Get("Etag")
		if etag == "" || path == "/main.js" {
			h := sha256.Sum256(rawBytes)
			etag = fmt.Sprintf(`W/"%x"`, h[:8])
		}

		var gzBuf bytes.Buffer
		gw, _ := gzip.NewWriterLevel(&gzBuf, gzip.BestSpeed)
		_, _ = gw.Write(rawBytes)
		_ = gw.Close()

		item = &desktopStaticCacheItem{
			contentType: cType,
			etag:        etag,
			rawBody:     rawBytes,
			gzipBody:    gzBuf.Bytes(),
		}

		desktopStaticCacheMu.Lock()
		desktopStaticCache[path] = item
		desktopStaticCacheMu.Unlock()
	}

	// 304 Not Modified validation
	if match := r.Header.Get("If-None-Match"); match != "" && (match == item.etag || match == "*") {
		w.Header().Set("ETag", item.etag)
		w.Header().Set("Cache-Control", "public, max-age=86400, stale-while-revalidate=604800")
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", item.contentType)
	w.Header().Set("ETag", item.etag)
	w.Header().Set("Cache-Control", "public, max-age=86400, stale-while-revalidate=604800")
	w.Header().Set("Vary", "Accept-Encoding")

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

// HandleDesktopIndex serves the official desktop web index.html with live CSRF injection,
// fast memory caching, and seamless iPad/tablet detection redirect.
func (p *Proxy) HandleDesktopIndex(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	port := p.activePort
	token := p.activeToken
	p.mu.RUnlock()

	if port == 0 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <title>Multigravity 启动中...</title>
  <link rel="icon" type="image/x-icon" href="/favicon.ico?v=3" />
  <link rel="icon" type="image/png" sizes="32x32" href="/icons/favicon-32.png?v=3" />
  <link rel="icon" type="image/png" sizes="192x192" href="/icons/icon-192.png?v=3" />
  <link rel="apple-touch-icon" href="/icons/icon-192.png?v=3" />
  <style>
    body { background: #131313; color: #e2e8f0; font-family: -apple-system, BlinkMacSystemFont, sans-serif; display: flex; flex-direction: column; align-items: center; justify-content: center; height: 100vh; margin: 0; }
    .spinner { width: 36px; height: 36px; border: 3px solid rgba(255,255,255,0.1); border-top-color: #38bdf8; border-radius: 50%; animation: spin 0.8s linear infinite; margin-bottom: 16px; }
    @keyframes spin { to { transform: rotate(360deg); } }
  </style>
</head>
<body>
  <div class="spinner"></div>
  <h2>正在连接 Multigravity 智能体服务...</h2>
  <p style="color: #94a3b8; font-size: 14px;">language_server 启动后将自动载入工作台</p>
  <script>setTimeout(() => location.reload(), 2000);</script>
</body>
</html>`))
		return
	}

	effectiveToken := token
	if effectiveToken == "" {
		effectiveToken = "headless-csrf-token"
	}

	// Fast path: serve cached transformed HTML if upstream port and token are unchanged
	desktopIndexCacheMu.RLock()
	if desktopIndexCachePort == port && desktopIndexCacheToken == token && len(desktopIndexCacheHTML) > 0 {
		cachedBytes := desktopIndexCacheHTML
		desktopIndexCacheMu.RUnlock()

		http.SetCookie(w, &http.Cookie{
			Name:     "csrfToken",
			Value:    effectiveToken,
			Path:     "/",
			SameSite: http.SameSiteLaxMode,
		})
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Content-Length", strconv.Itoa(len(cachedBytes)))
		w.WriteHeader(http.StatusOK)
		w.Write(cachedBytes)
		return
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
	reCSRF := regexp.MustCompile(`"csrfToken":"[^"]*"`)
	htmlStr = reCSRF.ReplaceAllString(htmlStr, fmt.Sprintf(`"csrfToken":%q`, effectiveToken))

	// Set cookie so ConnectRPC client and WebSocket can read it
	http.SetCookie(w, &http.Cookie{
		Name:     "csrfToken",
		Value:    effectiveToken,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
	})

	// 2. Align productName to Multigravity in window.__APP_CONFIG__
	reProduct := regexp.MustCompile(`"productName":"[^"]*"`)
	htmlStr = reProduct.ReplaceAllString(htmlStr, `"productName":"multigravity"`)

	// 3. Align page title
	reTitle := regexp.MustCompile(`(?i)<title>[^<]*</title>`)
	htmlStr = reTitle.ReplaceAllString(htmlStr, "<title>Multigravity</title>")

	// 4. Update html lang="en" to lang="zh-CN"
	htmlStr = strings.Replace(htmlStr, `<html lang="en">`, `<html lang="zh-CN">`, 1)

	// 5. Remove blocking external preconnect links to Google font & gstatic CDNs
	rePreconnect := regexp.MustCompile(`(?s)<link\s+rel="preconnect"\s+href="https?://(?:fonts\.|[a-z0-9-]+\.)?gstatic\.com[^"]*"\s*(?:crossorigin)?\s*/?>\s*`)
	htmlStr = rePreconnect.ReplaceAllString(htmlStr, "")
	reFontPreconnect := regexp.MustCompile(`(?s)<link\s+rel="preconnect"\s+href="https?://fonts\.googleapis\.com[^"]*"\s*(?:crossorigin)?\s*/?>\s*`)
	htmlStr = reFontPreconnect.ReplaceAllString(htmlStr, "")

	// 6. Cache-busting version parameter on script and stylesheet links to defeat stale browser disk cache
	htmlStr = strings.ReplaceAll(htmlStr, `src="/main.js"`, fmt.Sprintf(`src="/main.js?v=%s"`, desktopAssetVersion))
	htmlStr = strings.ReplaceAll(htmlStr, `src="/prism_bundle.js"`, fmt.Sprintf(`src="/prism_bundle.js?v=%s"`, desktopAssetVersion))
	htmlStr = strings.ReplaceAll(htmlStr, `href="/jetbox.css"`, fmt.Sprintf(`href="/jetbox.css?v=%s"`, desktopAssetVersion))
	htmlStr = strings.ReplaceAll(htmlStr, `href="/compiled_tailwind.css"`, fmt.Sprintf(`href="/compiled_tailwind.css?v=%s"`, desktopAssetVersion))

	// 7. Icons, metadata, tablet detection, telemetry stub, and O(1) text node interceptor
	multigravityIconsMeta := `    <link rel="icon" type="image/x-icon" href="/favicon.ico?v=3" />
    <link rel="icon" type="image/png" sizes="32x32" href="/icons/favicon-32.png?v=3" />
    <link rel="icon" type="image/png" sizes="192x192" href="/icons/icon-192.png?v=3" />
    <link rel="apple-touch-icon" href="/icons/icon-192.png?v=3" />
    <link rel="manifest" href="/manifest.json" />
    <meta name="apple-mobile-web-app-title" content="Multigravity" />
    <meta name="apple-mobile-web-app-capable" content="yes" />
    <meta name="apple-mobile-web-app-status-bar-style" content="black-translucent" />
    <meta name="theme-color" content="#0f172a" />
    <script>
      // Seamless iPadOS tablet detection: if masquerading as Mac desktop UA with touch points, redirect to PWA
      if (navigator.maxTouchPoints > 1 && !/Windows|Linux/.test(navigator.userAgent) && !location.search.includes("view=")) {
        location.replace("/?view=pwa");
      }
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
      // High-performance O(1) text node and attribute interceptor for dynamic strings
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
      })();
    </script>
    <script>` + desktopThemePresetFixJS + desktopRPCMuxJS + `</script>`
	reFavicon := regexp.MustCompile(`(?s)<link\s+(?:[^"'<>]|"[^"]*"|'[^']*')*rel=["'](?:shortcut\s+)?icon["'](?:[^"'<>]|"[^"]*"|'[^']*')*/?\s*>`)
	if reFavicon.MatchString(htmlStr) {
		htmlStr = reFavicon.ReplaceAllString(htmlStr, multigravityIconsMeta)
	} else {
		htmlStr = strings.Replace(htmlStr, "<head>", "<head>\n"+multigravityIconsMeta, 1)
	}

	htmlBytes := []byte(htmlStr)
	desktopIndexCacheMu.Lock()
	desktopIndexCachePort = port
	desktopIndexCacheToken = token
	desktopIndexCacheHTML = htmlBytes
	desktopIndexCacheMu.Unlock()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Content-Length", strconv.Itoa(len(htmlBytes)))
	w.WriteHeader(http.StatusOK)
	w.Write(htmlBytes)
}
