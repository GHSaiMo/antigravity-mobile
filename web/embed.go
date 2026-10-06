package web

import (
	"bufio"
	"compress/gzip"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
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
		strings.HasPrefix(ct, "text/event-stream") {
		g.skipGzip = true
		g.ResponseWriter.WriteHeader(status)
		return
	}
	g.ResponseWriter.Header().Del("Content-Length")
	g.ResponseWriter.Header().Set("Content-Encoding", "gzip")
	g.ResponseWriter.WriteHeader(status)
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
			setCacheHeaders(w, r, path)
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
			setCacheHeaders(w, r, "index.html")
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
