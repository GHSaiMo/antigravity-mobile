package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.2.0", "v1.2.0", 0},
		{"v1.2.1", "1.2.0", 1},
		{"1.2.0", "1.2.1", -1},
		{"v1.10.0", "v1.2.9", 1},
		{"v2.0.0", "v1.99.99", 1},
		{"1.2.0", "1.2.0-rc.1", 1},
		{"1.2.0-rc.1", "1.2.0", -1},
		{"1.2.0-rc.2", "1.2.0-rc.1", 1},
		{"1.2.0-rc.10", "1.2.0-rc.2", 1},
		{"1.2.0-beta.1", "1.2.0-alpha.9", 1},
	}
	for _, tc := range tests {
		if got := CompareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestEnsureTag(t *testing.T) {
	tag, err := EnsureTag("1.2.3")
	if err != nil || tag != "v1.2.3" {
		t.Fatalf("EnsureTag(1.2.3) = (%q, %v), want (v1.2.3, nil)", tag, err)
	}
	tag, err = EnsureTag("V1.2.3-rc.1")
	if err != nil || tag != "v1.2.3-rc.1" {
		t.Fatalf("EnsureTag(V1.2.3-rc.1) = (%q, %v), want (v1.2.3-rc.1, nil)", tag, err)
	}
	for _, bad := range []string{"", "../etc", "v1.2.0/foo", "v1.2.0;rm", "v1..2"} {
		if _, err := EnsureTag(bad); err == nil {
			t.Errorf("EnsureTag(%q) expected error, got nil", bad)
		}
	}
}

func TestParseChecksums(t *testing.T) {
	h1 := strings.Repeat("a", 64)
	h2 := strings.Repeat("B", 64)
	content := fmt.Sprintf(`
# comment line
%s  multigravity-darwin-arm64.tar.gz
%s *./multigravity-windows-amd64.zip
not-a-hash  broken.tar.gz
shortline
`, h1, h2)

	got := ParseChecksums(strings.NewReader(content))
	if len(got) != 2 {
		t.Fatalf("ParseChecksums got %d entries, want 2: %+v", len(got), got)
	}
	if got["multigravity-darwin-arm64.tar.gz"] != h1 {
		t.Errorf("unexpected darwin hash: %q", got["multigravity-darwin-arm64.tar.gz"])
	}
	if got["multigravity-windows-amd64.zip"] != strings.ToLower(h2) {
		t.Errorf("unexpected windows hash: %q", got["multigravity-windows-amd64.zip"])
	}
}

func TestAssetCandidates(t *testing.T) {
	c, err := AssetCandidates("darwin", "arm64")
	if err != nil || len(c) != 2 || c[0] != "multigravity-darwin-arm64.tar.gz" || c[1] != "multigravity-darwin-universal.tar.gz" {
		t.Fatalf("darwin/arm64 candidates = %v, %v", c, err)
	}
	c, err = AssetCandidates("linux", "amd64")
	if err != nil || len(c) != 1 || c[0] != "multigravity-linux-amd64.tar.gz" {
		t.Fatalf("linux/amd64 candidates = %v, %v", c, err)
	}
	c, err = AssetCandidates("windows", "arm64")
	if err != nil || len(c) != 1 || c[0] != "multigravity-windows-amd64.zip" {
		t.Fatalf("windows/arm64 candidates = %v, %v", c, err)
	}
	if _, err := AssetCandidates("plan9", "amd64"); err == nil {
		t.Errorf("expected error for unsupported OS")
	}
}

func TestFetchLatestTag_RedirectAndAPIFallback(t *testing.T) {
	// 1. Redirect works
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/org/repo/releases/latest" {
			http.Redirect(w, r, "https://github.com/org/repo/releases/tag/v1.3.0", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	tag, err := FetchLatestTag(context.Background(), Options{
		Repo:              "org/repo",
		GitHubBaseURL:     srv.URL,
		APIBaseURL:        srv.URL,
		MirrorPrefixes:    []string{},
		HTTPClientFactory: func(_ string, timeout time.Duration) *http.Client { return srv.Client() },
	})
	if err != nil || tag != "v1.3.0" {
		t.Fatalf("FetchLatestTag via redirect = (%q, %v), want (v1.3.0, nil)", tag, err)
	}

	// 2. Redirect fails -> falls back to GitHub API
	srvAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/org/repo/releases/latest" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"tag_name":"v1.4.2"}`))
			return
		}
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer srvAPI.Close()

	tag, err = FetchLatestTag(context.Background(), Options{
		Repo:              "org/repo",
		GitHubBaseURL:     srvAPI.URL,
		APIBaseURL:        srvAPI.URL,
		MirrorPrefixes:    []string{},
		HTTPClientFactory: func(_ string, timeout time.Duration) *http.Client { return srvAPI.Client() },
	})
	if err != nil || tag != "v1.4.2" {
		t.Fatalf("FetchLatestTag via API fallback = (%q, %v), want (v1.4.2, nil)", tag, err)
	}
}

func makeTarGz(t *testing.T, filename string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	hdr := &tar.Header{
		Name: filename,
		Mode: 0755,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("tar write header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("tar write content: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func makeZip(t *testing.T, filename string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(filename)
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestRun_TarGzUpdateWithChecksumAndSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script executable verification is Unix-only")
	}

	tmpDir := t.TempDir()
	realBin := filepath.Join(tmpDir, "real-mgy")
	linkBin := filepath.Join(tmpDir, "link-mgy")
	if err := os.WriteFile(realBin, []byte("#!/bin/sh\necho 'Multigravity (mgy) 1.2.0'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realBin, linkBin); err != nil {
		t.Fatal(err)
	}

	newScript := []byte("#!/bin/sh\necho 'Multigravity (mgy) 1.3.0'\n")
	goodArchive := makeTarGz(t, "mgy", newScript)
	badArchive := makeTarGz(t, "mgy", []byte("#!/bin/sh\necho 'tampered'\n"))

	assets, err := AssetCandidates(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatalf("AssetCandidates: %v", err)
	}
	assetName := assets[0]
	checksumLine := fmt.Sprintf("%s  %s\n", sha256Hex(goodArchive), assetName)

	// Serve tampered archive on /mirror/... and good archive on official path.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/org/repo/releases/latest":
			http.Redirect(w, r, "/org/repo/releases/tag/v1.3.0", http.StatusFound)
		case r.URL.Path == "/org/repo/releases/download/v1.3.0/checksums.txt":
			_, _ = w.Write([]byte(checksumLine))
		case strings.HasPrefix(r.URL.Path, "/mirror/") && strings.HasSuffix(r.URL.Path, "/"+assetName):
			_, _ = w.Write(badArchive)
		case r.URL.Path == "/org/repo/releases/download/v1.3.0/"+assetName:
			_, _ = w.Write(goodArchive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	res, err := Run(context.Background(), Options{
		CurrentVersion:    "1.2.0",
		Repo:              "org/repo",
		TargetPath:        linkBin,
		Out:               &out,
		GitHubBaseURL:     srv.URL,
		APIBaseURL:        srv.URL,
		MirrorPrefixes:    []string{srv.URL + "/mirror/"},
		HTTPClientFactory: func(_ string, _ time.Duration) *http.Client { return srv.Client() },
		SkipDarwinSign:    true,
	})
	if err != nil {
		t.Fatalf("Run failed: %v\noutput:\n%s", err, out.String())
	}
	if !res.Updated || !res.ChecksumVerified || res.LatestVersion != "1.3.0" {
		t.Fatalf("unexpected result: %+v", res)
	}
	// Symlink must be resolved and real binary updated in place.
	gotBytes, err := os.ReadFile(realBin)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotBytes) != string(newScript) {
		t.Fatalf("real binary not updated: got %q", string(gotBytes))
	}
	// Verify mirror tamper warning appeared and fallback succeeded.
	if !strings.Contains(out.String(), "SHA-256 校验不匹配") {
		t.Errorf("expected checksum mismatch fallback notice, got:\n%s", out.String())
	}
}

func TestRun_CheckOnlyAndFailClosedWithoutOfficialChecksums(t *testing.T) {
	tmpDir := t.TempDir()
	targetBin := filepath.Join(tmpDir, "mgy")
	if err := os.WriteFile(targetBin, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}

	archive := makeTarGz(t, "mgy", []byte("new-binary-bytes"))
	fakeChecksumLine := fmt.Sprintf("%s  multigravity-linux-amd64.tar.gz\n", sha256Hex(archive))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/org/repo/releases/latest":
			http.Redirect(w, r, "/org/repo/releases/tag/v1.2.0", http.StatusFound)
		case "/org/repo/releases/download/v1.2.0/multigravity-linux-amd64.tar.gz":
			_, _ = w.Write(archive)
		default:
			// Simulate compromised mirror trying to serve checksums.txt when official GitHub returns 404.
			if strings.HasPrefix(r.URL.Path, "/mirror/") && strings.HasSuffix(r.URL.Path, "/checksums.txt") {
				_, _ = w.Write([]byte(fakeChecksumLine))
				return
			}
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	baseOpts := Options{
		CurrentVersion:    "1.2.0",
		Repo:              "org/repo",
		GOOS:              "linux",
		GOARCH:            "amd64",
		TargetPath:        targetBin,
		Out:               &bytes.Buffer{},
		GitHubBaseURL:     srv.URL,
		APIBaseURL:        srv.URL,
		MirrorPrefixes:    []string{srv.URL + "/mirror/"},
		HTTPClientFactory: func(_ string, _ time.Duration) *http.Client { return srv.Client() },
		SkipVerifyExec:    true,
		SkipDarwinSign:    true,
	}

	// 1. CheckOnly on same version -> UpToDate, not Updated
	opts := baseOpts
	opts.CheckOnly = true
	res, err := Run(context.Background(), opts)
	if err != nil || !res.UpToDate || res.Updated {
		t.Fatalf("CheckOnly same version: res=%+v err=%v", res, err)
	}

	// 2. Normal Run on same version -> UpToDate no-op
	res, err = Run(context.Background(), baseOpts)
	if err != nil || !res.UpToDate || res.Updated {
		t.Fatalf("Normal same version: res=%+v err=%v", res, err)
	}

	// 3. Force Run without official checksums.txt (even if mirror has one) -> MUST fail closed (HIGH-02)
	opts = baseOpts
	opts.Force = true
	if _, err := Run(context.Background(), opts); err == nil {
		t.Fatalf("expected Force run without official checksums.txt to fail closed, got nil")
	}
	got, _ := os.ReadFile(targetBin)
	if string(got) != "old" {
		t.Fatalf("binary must remain untouched on checksum failure, got %q", string(got))
	}
}

func TestRun_WindowsZipUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	targetBin := filepath.Join(tmpDir, "mgy.exe")
	if err := os.WriteFile(targetBin, []byte("old-exe"), 0755); err != nil {
		t.Fatal(err)
	}

	winZip := makeZip(t, "mgy.exe", []byte("new-windows-exe"))
	winChecksum := fmt.Sprintf("%s  multigravity-windows-amd64.zip\n", sha256Hex(winZip))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/org/repo/releases/download/v1.2.1/checksums.txt":
			_, _ = w.Write([]byte(winChecksum))
		case "/org/repo/releases/download/v1.2.1/multigravity-windows-amd64.zip":
			_, _ = w.Write(winZip)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	res, err := Run(context.Background(), Options{
		CurrentVersion:    "1.2.0",
		TargetVersion:     "v1.2.1",
		Repo:              "org/repo",
		GOOS:              "windows",
		GOARCH:            "amd64",
		TargetPath:        targetBin,
		Out:               &bytes.Buffer{},
		GitHubBaseURL:     srv.URL,
		APIBaseURL:        srv.URL,
		MirrorPrefixes:    []string{},
		HTTPClientFactory: func(_ string, _ time.Duration) *http.Client { return srv.Client() },
		SkipVerifyExec:    true,
		SkipDarwinSign:    true,
	})
	if err != nil {
		t.Fatalf("Windows zip update failed: %v", err)
	}
	if !res.Updated || res.AssetName != "multigravity-windows-amd64.zip" {
		t.Fatalf("unexpected result: %+v", res)
	}
	got, err := os.ReadFile(targetBin)
	if err != nil || string(got) != "new-windows-exe" {
		t.Fatalf("windows binary = %q, err=%v", string(got), err)
	}
}

func TestBackgroundUpdateCheckCache(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "update_check.json")
	baseNow := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)

	calls := 0
	fetch := func(_ context.Context) (string, error) {
		calls++
		return "v1.3.0", nil
	}

	// 1. Cache miss -> fetches and reports update
	tag, hasUpdate := checkForUpdateWithCache(context.Background(), "1.2.0", cachePath, baseNow, fetch)
	if !hasUpdate || tag != "v1.3.0" || calls != 1 {
		t.Fatalf("first check: tag=%q hasUpdate=%v calls=%d", tag, hasUpdate, calls)
	}

	// 2. Cache hit within 12h -> no fetch, still reports update for old version
	tag, hasUpdate = checkForUpdateWithCache(context.Background(), "1.2.0", cachePath, baseNow.Add(2*time.Hour), fetch)
	if !hasUpdate || tag != "v1.3.0" || calls != 1 {
		t.Fatalf("cached check (old ver): tag=%q hasUpdate=%v calls=%d", tag, hasUpdate, calls)
	}

	// 3. Cache hit after user upgraded -> no fetch, no update
	tag, hasUpdate = checkForUpdateWithCache(context.Background(), "1.3.0", cachePath, baseNow.Add(3*time.Hour), fetch)
	if hasUpdate || tag != "v1.3.0" || calls != 1 {
		t.Fatalf("cached check (new ver): tag=%q hasUpdate=%v calls=%d", tag, hasUpdate, calls)
	}

	// 4. Expired cache (>12h) -> fetches again
	tag, hasUpdate = checkForUpdateWithCache(context.Background(), "1.3.0", cachePath, baseNow.Add(13*time.Hour), fetch)
	if hasUpdate || tag != "v1.3.0" || calls != 2 {
		t.Fatalf("expired check: tag=%q hasUpdate=%v calls=%d", tag, hasUpdate, calls)
	}
}

func TestIsBackgroundCheckDisabled(t *testing.T) {
	t.Setenv("MULTIGRAVITY_NO_UPDATE_CHECK", "")
	t.Setenv("MULTIGRAVITY_UPDATE_CHECK", "")
	if IsBackgroundCheckDisabled() {
		t.Errorf("expected enabled by default")
	}

	t.Setenv("MULTIGRAVITY_UPDATE_CHECK", "0")
	if !IsBackgroundCheckDisabled() {
		t.Errorf("expected disabled when MULTIGRAVITY_UPDATE_CHECK=0")
	}

	t.Setenv("MULTIGRAVITY_UPDATE_CHECK", "")
	t.Setenv("MULTIGRAVITY_NO_UPDATE_CHECK", "1")
	if !IsBackgroundCheckDisabled() {
		t.Errorf("expected disabled when MULTIGRAVITY_NO_UPDATE_CHECK=1")
	}
}
