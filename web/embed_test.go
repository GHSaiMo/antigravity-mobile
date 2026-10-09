package web

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func get(t *testing.T, h http.Handler, path string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHandlerETagAndGzip(t *testing.T) {
	h := Handler()

	plain := get(t, h, "/style.css", nil)
	if plain.Code != 200 || plain.Header().Get("ETag") == "" || plain.Header().Get("Content-Encoding") != "" {
		t.Fatalf("plain: code=%d etag=%q enc=%q", plain.Code, plain.Header().Get("ETag"), plain.Header().Get("Content-Encoding"))
	}

	gz := get(t, h, "/style.css", map[string]string{"Accept-Encoding": "gzip"})
	if gz.Header().Get("Content-Encoding") != "gzip" || gz.Header().Get("Content-Length") == "" {
		t.Fatalf("gzip headers: %v", gz.Header())
	}
	zr, err := gzip.NewReader(gz.Body)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(zr)
	if string(got) != plain.Body.String() {
		t.Fatal("gunzipped body differs from identity body")
	}
	if gz.Header().Get("ETag") == plain.Header().Get("ETag") {
		t.Fatal("gzip and identity representations must have distinct ETags")
	}

	// Conditional request -> 304 with no body.
	cond := get(t, h, "/style.css", map[string]string{"Accept-Encoding": "gzip", "If-None-Match": gz.Header().Get("ETag")})
	if cond.Code != http.StatusNotModified || cond.Body.Len() != 0 {
		t.Fatalf("conditional: code=%d bodyLen=%d", cond.Code, cond.Body.Len())
	}

	// Range bypasses gzip.
	rng := get(t, h, "/style.css", map[string]string{"Accept-Encoding": "gzip", "Range": "bytes=0-9"})
	if rng.Code != http.StatusPartialContent || rng.Header().Get("Content-Encoding") != "" {
		t.Fatalf("range: code=%d enc=%q", rng.Code, rng.Header().Get("Content-Encoding"))
	}
}

func TestHandlerFallbackAnd404(t *testing.T) {
	h := Handler()
	if rec := get(t, h, "/some/spa/route", nil); rec.Code != 200 {
		t.Fatalf("spa fallback: %d", rec.Code)
	}
	if rec := get(t, h, "/missing.js", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("missing asset: %d", rec.Code)
	}
	if rec := get(t, h, "/", nil); rec.Header().Get("Cache-Control") != "no-cache, must-revalidate" {
		t.Fatalf("index cache-control: %q", rec.Header().Get("Cache-Control"))
	}
}
