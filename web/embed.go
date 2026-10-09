package web

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed index.html style.css js manifest.json sw.js icons mermaid.min.js favicon.ico
var staticFiles embed.FS

var gzipPool = sync.Pool{
	New: func() interface{} {
		w, _ := gzip.NewWriterLevel(io.Discard, gzip.BestSpeed)
		return w
	},
}

type gzipResponseWriter struct {
	http.ResponseWriter
	writer      *gzip.Writer
	wroteHeader bool
	skipGzip    bool
}

func (g *gzipResponseWriter) WriteHeader(status int) {
	if g.wroteHeader {
		return
	}
	g.wroteHeader = true
	ct := strings.ToLower(g.ResponseWriter.Header().Get("Content-Type"))
	if status == http.StatusNoContent || status == http.StatusNotModified ||
		g.ResponseWriter.Header().Get("Content-Encoding") != "" ||
		strings.HasPrefix(ct, "application/connect+") ||
		strings.HasPrefix(ct, "application/grpc") ||
		strings.HasPrefix(ct, "text/event-stream") ||
		alreadyCompressedType(ct) ||
		knownSmallBody(g.ResponseWriter.Header().Get("Content-Length")) {
		g.skipGzip = true
		g.ResponseWriter.WriteHeader(status)
		return
	}
	g.ResponseWriter.Header().Del("Content-Length")
	g.ResponseWriter.Header().Set("Content-Encoding", "gzip")
	g.ResponseWriter.WriteHeader(status)
}

// gzipMinSize is the smallest body worth compressing: below it the gzip header, trailer and chunked
// framing outweigh the savings (e.g. a 500 B status reply grows to ~1.2 KB on the wire).
const gzipMinSize = 1024

func knownSmallBody(contentLength string) bool {
	n, err := strconv.Atoi(contentLength)
	return err == nil && n < gzipMinSize
}

// alreadyCompressedType reports content types that gzip cannot shrink further.
func alreadyCompressedType(ct string) bool {
	if strings.HasPrefix(ct, "image/") {
		return ct != "image/svg+xml" && !strings.HasPrefix(ct, "image/svg+xml;") && !strings.HasPrefix(ct, "image/x-icon") && !strings.HasPrefix(ct, "image/bmp")
	}
	if strings.HasPrefix(ct, "video/") || strings.HasPrefix(ct, "audio/") || strings.HasPrefix(ct, "font/woff") {
		return true
	}
	switch {
	case strings.HasPrefix(ct, "application/zip"), strings.HasPrefix(ct, "application/gzip"),
		strings.HasPrefix(ct, "application/x-gzip"), strings.HasPrefix(ct, "application/pdf"),
		strings.HasPrefix(ct, "application/x-7z"), strings.HasPrefix(ct, "application/x-rar"),
		strings.HasPrefix(ct, "application/x-xz"), strings.HasPrefix(ct, "application/zstd"):
		return true
	}
	return false
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	if !g.wroteHeader {
		g.WriteHeader(http.StatusOK)
	}
	if g.skipGzip {
		return g.ResponseWriter.Write(b)
	}
	return g.writer.Write(b)
}

func (g *gzipResponseWriter) Flush() {
	if !g.skipGzip && g.writer != nil {
		_ = g.writer.Flush()
	}
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (g *gzipResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := g.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("response writer does not support hijacking")
}

// staticAsset is one embedded file with its validator and (lazily built) gzip variant.
// embed.FS reports a zero ModTime, so http.FileServer can never answer conditional
// requests; we derive a content-hash ETag instead and compress each file only once.
type staticAsset struct {
	name     string
	raw      []byte
	etag     string
	ctype    string
	gzOnce   sync.Once
	gzipBody []byte
}

var (
	assetsMu sync.RWMutex
	assets   = map[string]*staticAsset{}
)

// compressibleAsset reports whether gzip is worthwhile for the file extension.
func compressibleAsset(ext string) bool {
	switch ext {
	case ".html", ".css", ".js", ".json", ".svg", ".ico", ".txt", ".xml":
		return true
	}
	return false
}

func loadAsset(path string) (*staticAsset, bool) {
	assetsMu.RLock()
	a, ok := assets[path]
	assetsMu.RUnlock()
	if ok {
		return a, true
	}
	raw, err := staticFiles.ReadFile(path)
	if err != nil {
		return nil, false
	}
	sum := sha256.Sum256(raw)
	ext := strings.ToLower(filepath.Ext(path))
	ctype := mime.TypeByExtension(ext)
	if ctype == "" {
		ctype = http.DetectContentType(raw)
	}
	a = &staticAsset{
		name:  path,
		raw:   raw,
		etag:  `"` + hex.EncodeToString(sum[:8]) + `"`,
		ctype: ctype,
	}
	assetsMu.Lock()
	if existing, ok := assets[path]; ok {
		a = existing
	} else {
		assets[path] = a
	}
	assetsMu.Unlock()
	return a, true
}

// gzipped returns the cached gzip variant, or nil when it would not be smaller.
func (a *staticAsset) gzipped() []byte {
	a.gzOnce.Do(func() {
		if !compressibleAsset(strings.ToLower(filepath.Ext(a.name))) || len(a.raw) < 1024 {
			return
		}
		var buf bytes.Buffer
		gw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		_, _ = gw.Write(a.raw)
		_ = gw.Close()
		if buf.Len() < len(a.raw) {
			a.gzipBody = buf.Bytes()
		}
	})
	return a.gzipBody
}

func serveAsset(w http.ResponseWriter, r *http.Request, a *staticAsset) {
	h := w.Header()
	h.Set("ETag", a.etag)
	h.Set("Content-Type", a.ctype)
	h.Add("Vary", "Accept-Encoding")

	body := a.raw
	// Range requests address the identity representation, so only use gzip for full-body GETs.
	if r.Header.Get("Range") == "" && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		if gz := a.gzipped(); gz != nil {
			body = gz
			h.Set("Content-Encoding", "gzip")
			// ServeContent only sets Content-Length for identity bodies.
			h.Set("Content-Length", strconv.Itoa(len(gz)))
			// Distinct validator per representation, as required for shared caches.
			h.Set("ETag", strings.TrimSuffix(a.etag, `"`)+`-gz"`)
		}
	}
	// ServeContent handles If-None-Match (304), HEAD, Range and Content-Length.
	http.ServeContent(w, r, a.name, time.Time{}, bytes.NewReader(body))
}

// Handler returns an http.Handler that serves embedded web assets with content-hash ETags
// (so revalidation costs a 304), pre-compressed gzip, proper cache headers, and robust SPA fallback.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		if a, ok := loadAsset(path); ok {
			setCacheHeaders(w, r, path)
			serveAsset(w, r, a)
			return
		}

		// Known static file extensions that should 404 when not found
		ext := strings.ToLower(filepath.Ext(path))
		isStaticAsset := ext == ".js" || ext == ".css" || ext == ".png" || ext == ".svg" ||
			ext == ".ico" || ext == ".json" || ext == ".woff" || ext == ".woff2" || ext == ".map"

		if !isStaticAsset {
			// SPA route fallback to index.html
			if a, ok := loadAsset("index.html"); ok {
				setCacheHeaders(w, r, "index.html")
				serveAsset(w, r, a)
				return
			}
		}

		http.NotFound(w, r)
	})
}

// GzipHandler compresses HTTP responses with gzip if the client supports it.
func GzipHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") ||
			r.Header.Get("Range") != "" || // a compressed body would not match the requested byte range
			strings.Contains(strings.ToLower(r.Header.Get("Upgrade")), "websocket") ||
			r.Header.Get("Sec-WebSocket-Key") != "" ||
			strings.Contains(r.URL.Path, "Stream") ||
			strings.Contains(r.URL.Path, "Subscribe") ||
			strings.Contains(r.URL.Path, "Watch") {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Add("Vary", "Accept-Encoding")
		gz := gzipPool.Get().(*gzip.Writer)
		defer gzipPool.Put(gz)

		gz.Reset(w)

		gzw := &gzipResponseWriter{ResponseWriter: w, writer: gz}
		defer func() {
			if !gzw.skipGzip {
				_ = gz.Close()
			}
		}()

		next.ServeHTTP(gzw, r)
	})
}

func setCacheHeaders(w http.ResponseWriter, r *http.Request, path string) {
	// Service worker, HTML shell must always revalidate
	if path == "sw.js" || path == "index.html" || path == "" {
		w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		return
	}

	// Assets requested with explicit cache-busting version parameter (?v=...) can be cached immutably
	if r != nil && r.URL.Query().Get("v") != "" {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}

	// Large immutable assets like mermaid.min.js or icons can be cached for a week
	if path == "mermaid.min.js" || strings.HasPrefix(path, "icons/") {
		w.Header().Set("Cache-Control", "public, max-age=604800") // 7 days
		return
	}

	// General CSS/JS: default to short revalidation window
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")
}

// GetFS returns the underlying embedded filesystem.
func GetFS() fs.FS {
	return staticFiles
}
