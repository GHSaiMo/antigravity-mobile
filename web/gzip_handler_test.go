package web

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func gzipResponse(t *testing.T, ct string, body string, setLength bool) *httptest.ResponseRecorder {
	t.Helper()
	h := GzipHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", ct)
		if setLength {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(body))
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestGzipHandlerSkipsSmallAndCompressedBodies(t *testing.T) {
	big := strings.Repeat(`{"k":"value"},`, 500)
	cases := []struct {
		name      string
		ct        string
		body      string
		setLength bool
		wantGzip  bool
	}{
		{"large json", "application/json", big, true, true},
		{"large json without length", "application/json", big, false, true},
		{"small json with length", "application/json", `{"status":"ok"}`, true, false},
		{"jpeg", "image/jpeg", big, true, false},
		{"png", "image/png", big, false, false},
		{"svg is text", "image/svg+xml", big, true, true},
		{"zip", "application/zip", big, true, false},
		{"pdf", "application/pdf", big, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := gzipResponse(t, tc.ct, tc.body, tc.setLength)
			gotGzip := rec.Header().Get("Content-Encoding") == "gzip"
			if gotGzip != tc.wantGzip {
				t.Fatalf("gzip=%v, want %v", gotGzip, tc.wantGzip)
			}
			if !gotGzip {
				if rec.Body.String() != tc.body {
					t.Fatal("uncompressed body was altered")
				}
				if tc.setLength && rec.Header().Get("Content-Length") != strconv.Itoa(len(tc.body)) {
					t.Fatal("Content-Length must be kept for uncompressed bodies")
				}
			}
		})
	}
}

func TestGzipHandlerSkipsRangeRequests(t *testing.T) {
	h := GzipHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusPartialContent)
		w.Write([]byte(strings.Repeat("a", 4096)))
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/raw", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Range", "bytes=0-4095")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Content-Encoding") != "" {
		t.Fatal("range responses must not be gzipped")
	}
}
