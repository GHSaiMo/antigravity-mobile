package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed index.html style.css app.js manifest.json sw.js icons
var staticFiles embed.FS

// Handler returns an http.Handler that serves the embedded web assets with SPA fallback to index.html.
func Handler() http.Handler {
	fileServer := http.FileServer(http.FS(staticFiles))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		// Try opening the requested file in the embedded FS
		f, err := staticFiles.Open(path)
		if err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// If not found and not a static asset (no extension), serve index.html for SPA
		if !strings.Contains(path, ".") {
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}

		http.NotFound(w, r)
	})
}

// GetFS returns the underlying embedded filesystem.
func GetFS() fs.FS {
	return staticFiles
}
