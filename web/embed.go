package web

import (
	"compress/gzip"
	"embed"
	"io"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed index.html style.css app.js manifest.json sw.js icons mermaid.min.js zh-CN.js view-switcher.js view-switcher.css
var staticFiles embed.FS

var gzipPool = sync.Pool{
	New: func() interface{} {
		w, _ := gzip.NewWriterLevel(io.Discard, gzip.BestSpeed)
		return w
	},
}

type gzipResponseWriter struct {
	http.ResponseWriter
	writer *gzip.Writer
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	return g.writer.Write(b)
}

func (g *gzipResponseWriter) WriteHeader(status int) {
	g.ResponseWriter.WriteHeader(status)
}

// Handler returns an http.Handler that serves embedded web assets with gzip compression,
// proper cache headers, and robust SPA fallback.
func Handler() http.Handler {
	fileServer := http.FileServer(http.FS(staticFiles))

	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		// Try opening the requested file in the embedded FS
		f, err := staticFiles.Open(path)
		if err == nil {
			f.Close()
			setCacheHeaders(w, path)
			fileServer.ServeHTTP(w, r)
			return
		}

		// Known static file extensions that should 404 when not found
		ext := strings.ToLower(filepath.Ext(path))
		isStaticAsset := ext == ".js" || ext == ".css" || ext == ".png" || ext == ".svg" ||
			ext == ".ico" || ext == ".json" || ext == ".woff" || ext == ".woff2" || ext == ".map"

		if !isStaticAsset {
			// SPA route fallback to index.html
			r.URL.Path = "/"
			setCacheHeaders(w, "index.html")
			fileServer.ServeHTTP(w, r)
			return
		}

		http.NotFound(w, r)
	})

	return GzipHandler(baseHandler)
}

// GzipHandler compresses HTTP responses with gzip if the client supports it.
func GzipHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") ||
			strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Add("Vary", "Accept-Encoding")
		gz := gzipPool.Get().(*gzip.Writer)
		defer gzipPool.Put(gz)

		gz.Reset(w)
		defer gz.Close()

		w.Header().Set("Content-Encoding", "gzip")
		gzw := &gzipResponseWriter{ResponseWriter: w, writer: gz}
		next.ServeHTTP(gzw, r)
	})
}

func setCacheHeaders(w http.ResponseWriter, path string) {
	// Service worker and HTML shell must not be cached aggressively
	if path == "sw.js" || path == "index.html" || path == "" {
		w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		return
	}

	// Large immutable assets like mermaid.min.js or icons can be cached
	if path == "mermaid.min.js" || strings.HasPrefix(path, "icons/") {
		w.Header().Set("Cache-Control", "public, max-age=604800") // 7 days
		return
	}

	// General CSS/JS: cache with validation
	w.Header().Set("Cache-Control", "public, max-age=86400") // 1 day
}

// GetFS returns the underlying embedded filesystem.
func GetFS() fs.FS {
	return staticFiles
}
