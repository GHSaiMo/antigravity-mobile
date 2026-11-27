package updater

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/i18n"
	"antigravity-mobile/internal/localtls"
	"antigravity-mobile/internal/procutil"
)

const (
	DefaultRepo          = "GHSaiMo/antigravity-mobile"
	DefaultGitHubBaseURL = "https://github.com"
	DefaultAPIBaseURL    = "https://api.github.com"

	maxBinarySizeBytes  = 256 * 1024 * 1024 // 256 MiB safety cap
	maxArchiveSizeBytes = 256 * 1024 * 1024 // 256 MiB safety cap
	userAgent           = "Multigravity-Updater/1.0"
)

// DefaultMirrorPrefixes mirrors the download strategy used by scripts/install.sh and internal/tunnel/cloudflare.go.
var DefaultMirrorPrefixes = []string{
	"https://ghfast.top/",
	"https://ghproxy.net/",
}

var (
	repoPattern   = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	sha256HexRe   = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
	releaseTagRe  = regexp.MustCompile(`/releases/tag/([^/?#]+)`)
	errNotFound   = errors.New("not found")
)

// Options configures an update check or self-update run.
type Options struct {
	CurrentVersion string
	TargetVersion  string // Optional specific version/tag (e.g. "v1.2.0"); empty means latest release.
	Repo           string // Defaults to MULTIGRAVITY_REPO or DefaultRepo.
	CheckOnly      bool   // Only check for updates without downloading or replacing.
	Force          bool   // Reinstall even if already on the target/latest version.
	TargetPath     string // Optional override for executable path (defaults to current binary).
	Out            io.Writer
	Lang           string                     // Optional UI language ("zh", "en", "ja"); empty falls back to gateway i18n.GetLang().
	OnProgress     func(stage, detail string) // Optional stage callback ("checking", "downloading", "verifying", "replacing").

	// Overridable fields for testing and platform simulation.
	GOOS              string
	GOARCH            string
	GitHubBaseURL     string
	APIBaseURL        string
	MirrorPrefixes    []string
	HTTPClientFactory func(rawURL string, timeout time.Duration) *http.Client
	SkipVerifyExec    bool
	SkipDarwinSign    bool
}

// Result describes the outcome of a Run invocation.
type Result struct {
	CurrentVersion   string
	LatestVersion    string
	Tag              string
	AssetName        string
	DownloadURL      string
	TargetPath       string
	Updated          bool
	UpToDate         bool
	ChecksumVerified bool
	ChecksumSkipped  bool
}

// RepoFromEnv returns MULTIGRAVITY_REPO if valid, otherwise DefaultRepo.
func RepoFromEnv() string {
	raw := strings.TrimSpace(os.Getenv("MULTIGRAVITY_REPO"))
	if raw != "" && repoPattern.MatchString(raw) && !strings.Contains(raw, "..") {
		return raw
	}
	return DefaultRepo
}

func (o *Options) withDefaults() Options {
	cp := *o
	if cp.Repo == "" {
		cp.Repo = RepoFromEnv()
	}
	if cp.GOOS == "" {
		cp.GOOS = runtime.GOOS
	}
	if cp.GOARCH == "" {
		cp.GOARCH = runtime.GOARCH
	}
	if cp.GitHubBaseURL == "" {
		cp.GitHubBaseURL = DefaultGitHubBaseURL
	}
	cp.GitHubBaseURL = strings.TrimRight(cp.GitHubBaseURL, "/")
	if cp.APIBaseURL == "" {
		cp.APIBaseURL = DefaultAPIBaseURL
	}
	cp.APIBaseURL = strings.TrimRight(cp.APIBaseURL, "/")
	if cp.MirrorPrefixes == nil {
		cp.MirrorPrefixes = append([]string(nil), DefaultMirrorPrefixes...)
	}
	if cp.Out == nil {
		cp.Out = os.Stdout
	}
	return cp
}

// AssetCandidates returns the ordered release asset filenames for the given OS/arch.
func AssetCandidates(goos, goarch string) ([]string, error) {
	return assetCandidatesForLang(goos, goarch, i18n.GetLang())
}

func assetCandidatesForLang(goos, goarch, lang string) ([]string, error) {
	l := i18n.ResolveLang(lang)
	switch goos {
	case "darwin":
		switch goarch {
		case "arm64":
			return []string{"multigravity-darwin-arm64.tar.gz", "multigravity-darwin-universal.tar.gz"}, nil
		case "amd64":
			return []string{"multigravity-darwin-amd64.tar.gz", "multigravity-darwin-universal.tar.gz"}, nil
		default:
			return []string{"multigravity-darwin-universal.tar.gz"}, nil
		}
	case "linux":
		switch goarch {
		case "amd64":
			return []string{"multigravity-linux-amd64.tar.gz"}, nil
		case "arm64":
			return []string{"multigravity-linux-arm64.tar.gz"}, nil
		default:
			return nil, fmt.Errorf(i18n.T3For(l,
				"暂不支持 Linux %s 架构的预编译包",
				"prebuilt binary for Linux %s is not supported yet",
				"Linux %s 向けのビルド済みパッケージは未対応です",
			), goarch)
		}
	case "windows":
		switch goarch {
		case "amd64", "arm64":
			return []string{"multigravity-windows-amd64.zip"}, nil
		default:
			return nil, fmt.Errorf(i18n.T3For(l,
				"暂不支持 Windows %s 架构的预编译包",
				"prebuilt binary for Windows %s is not supported yet",
				"Windows %s 向けのビルド済みパッケージは未対応です",
			), goarch)
		}
	default:
		return nil, fmt.Errorf(i18n.T3For(l,
			"暂不支持 %s/%s 平台的自动更新",
			"auto-update is not supported on %s/%s",
			"%s/%s プラットフォームの自動更新は未対応です",
		), goos, goarch)
	}
}

// detectLocalProxy checks environment variables and common local proxy ports (with 60s caching).
func detectLocalProxy() *url.URL {
	return config.DetectLocalProxy()
}

func isOfficialGitHubURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "github.com" || host == "api.github.com" || host == "objects.githubusercontent.com" || host == "release-assets.githubusercontent.com"
}

func (o Options) newHTTPClient(rawURL string, timeout time.Duration) *http.Client {
	if o.HTTPClientFactory != nil {
		return o.HTTPClientFactory(rawURL, timeout)
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		IdleConnTimeout:       60 * time.Second,
	}

	if isOfficialGitHubURL(rawURL) {
		if proxyURL := detectLocalProxy(); proxyURL != nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		} else {
			transport.Proxy = http.ProxyFromEnvironment
		}
	} else {
		// Domestic mirrors connect directly to avoid proxy interference.
		transport.Proxy = nil
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
}

func cloneClientWithoutRedirect(base *http.Client) *http.Client {
	cp := *base
	cp.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &cp
}

// FetchLatestTag discovers the latest release tag without hitting GitHub API rate limits when possible.
func FetchLatestTag(ctx context.Context, opts Options) (string, error) {
	o := opts.withDefaults()
	var errs []string

	// 1. Official GitHub /releases/latest redirect (zero API quota cost)
	latestURL := fmt.Sprintf("%s/%s/releases/latest", o.GitHubBaseURL, o.Repo)
	if tag, err := fetchTagViaRedirect(ctx, o, latestURL, 10*time.Second); err == nil {
		return tag, nil
	} else {
		errs = append(errs, fmt.Sprintf("github redirect: %v", err))
	}

	// 2. Official GitHub Releases API fallback (strictly official GitHub; never trust third-party mirrors for tag resolution)
	apiURL := fmt.Sprintf("%s/repos/%s/releases/latest", o.APIBaseURL, o.Repo)
	if tag, err := fetchTagViaAPI(ctx, o, apiURL, 10*time.Second); err == nil {
		return tag, nil
	} else {
		errs = append(errs, fmt.Sprintf("github api: %v", err))
	}

	l := i18n.ResolveLang(o.Lang)
	return "", fmt.Errorf(i18n.T3For(l,
		"无法获取最新版本信息 (%s)",
		"failed to fetch latest release info (%s)",
		"最新バージョン情報を取得できませんでした (%s)",
	), strings.Join(errs, "; "))
}

func extractTagFromURL(rawURL string) (string, error) {
	m := releaseTagRe.FindStringSubmatch(rawURL)
	if len(m) < 2 {
		return "", fmt.Errorf("no /releases/tag/ segment in %q", rawURL)
	}
	unescaped, err := url.PathUnescape(m[1])
	if err != nil {
		unescaped = m[1]
	}
	return EnsureTag(unescaped)
}

func fetchTagViaRedirect(ctx context.Context, o Options, targetURL string, timeout time.Duration) (string, error) {
	client := cloneClientWithoutRedirect(o.newHTTPClient(targetURL, timeout))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		loc := resp.Header.Get("Location")
		if tag, err := extractTagFromURL(loc); err == nil {
			return tag, nil
		}
		return "", fmt.Errorf("unexpected redirect location: %q", loc)
	}
	if resp.StatusCode == http.StatusOK && resp.Request != nil && resp.Request.URL != nil {
		if tag, err := extractTagFromURL(resp.Request.URL.String()); err == nil {
			return tag, nil
		}
	}
	return "", fmt.Errorf("HTTP %d", resp.StatusCode)
}

func fetchTagViaAPI(ctx context.Context, o Options, apiURL string, timeout time.Duration) (string, error) {
	client := o.newHTTPClient(apiURL, timeout)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return "", err
	}
	return EnsureTag(payload.TagName)
}

// ParseChecksums parses sha256sum output ("<64-hex>  <filename>" or "<64-hex> *<filename>").
func ParseChecksums(r io.Reader) map[string]string {
	result := make(map[string]string)
	scanner := bufio.NewScanner(io.LimitReader(r, 1<<20))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		hash := strings.ToLower(strings.TrimSpace(fields[0]))
		if !sha256HexRe.MatchString(hash) {
			continue
		}
		name := strings.TrimSpace(fields[1])
		name = strings.TrimPrefix(name, "*")
		name = strings.TrimPrefix(name, "./")
		name = filepath.Base(name)
		if name != "" && name != "." {
			result[name] = hash
		}
	}
	return result
}

// FetchChecksums fetches checksums.txt exclusively from the official GitHub HTTPS endpoint (HIGH-02).
// Third-party mirrors are intentionally never queried for checksums so a compromised mirror cannot
// serve both a tampered archive and a matching checksums.txt.
func FetchChecksums(ctx context.Context, tag string, opts Options) (map[string]string, bool, error) {
	o := opts.withDefaults()
	ghURL := fmt.Sprintf("%s/%s/releases/download/%s/checksums.txt", o.GitHubBaseURL, o.Repo, tag)
	if o.HTTPClientFactory == nil && !strings.HasPrefix(strings.ToLower(ghURL), "https://") {
		return nil, false, fmt.Errorf("refusing non-HTTPS checksum URL: %s", ghURL)
	}

	sums, err := fetchChecksumsFromURL(ctx, o, ghURL)
	if err == nil && len(sums) > 0 {
		return sums, true, nil
	}
	if errors.Is(err, errNotFound) {
		return nil, false, fmt.Errorf("official release %s is missing checksums.txt", tag)
	}
	if err != nil {
		return nil, false, fmt.Errorf("official checksums.txt fetch failed: %w", err)
	}
	return nil, false, fmt.Errorf("checksums.txt unavailable")
}

func fetchChecksumsFromURL(ctx context.Context, o Options, rawURL string) (map[string]string, error) {
	client := o.newHTTPClient(rawURL, 8*time.Second)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	sums := ParseChecksums(resp.Body)
	if len(sums) == 0 {
		return nil, fmt.Errorf("no valid checksum entries")
	}
	return sums, nil
}

// ResolveExecutablePath resolves the real on-disk executable path (following symlinks).
func ResolveExecutablePath(override string) (string, error) {
	exePath := strings.TrimSpace(override)
	if exePath == "" {
		var err error
		exePath, err = os.Executable()
		if err != nil {
			return "", fmt.Errorf(i18n.T3(
				"无法定位当前可执行文件路径: %w",
				"failed to locate current executable path: %w",
				"現在の実行ファイルパスを特定できませんでした: %w",
			), err)
		}
	}
	resolved, err := filepath.EvalSymlinks(exePath)
	if err == nil && resolved != "" {
		exePath = resolved
	}
	abs, err := filepath.Abs(exePath)
	if err == nil {
		exePath = abs
	}
	return exePath, nil
}

func checkDirWritable(dir string) error {
	probe, err := os.CreateTemp(dir, ".mgy-write-check-*")
	if err != nil {
		return err
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)
	return nil
}

// SelfUpdateCapability reports whether the running mgy binary can safely self-update in place.
type SelfUpdateCapability struct {
	CanUpdate  bool
	Reason     string
	TargetPath string
	Dir        string
}

// InspectSelfUpdateCapability checks platform support, binary path, and directory writability
// without performing any network request.
func InspectSelfUpdateCapability(targetPathOverride, currentVersion string) SelfUpdateCapability {
	return InspectSelfUpdateCapabilityForLang(targetPathOverride, currentVersion, i18n.GetLang())
}

// InspectSelfUpdateCapabilityForLang checks self-update capability and formats Reason for lang ("zh"/"en"/"ja").
func InspectSelfUpdateCapabilityForLang(targetPathOverride, currentVersion, lang string) SelfUpdateCapability {
	l := i18n.ResolveLang(lang)
	if _, err := assetCandidatesForLang(runtime.GOOS, runtime.GOARCH, l); err != nil {
		return SelfUpdateCapability{
			CanUpdate: false,
			Reason:    err.Error(),
		}
	}
	targetPath, err := ResolveExecutablePath(targetPathOverride)
	if err != nil {
		return SelfUpdateCapability{
			CanUpdate: false,
			Reason:    err.Error(),
		}
	}
	targetDir := filepath.Dir(targetPath)
	cap := SelfUpdateCapability{
		CanUpdate:  true,
		TargetPath: targetPath,
		Dir:        targetDir,
	}

	slashPath := filepath.ToSlash(targetPath)
	if strings.Contains(slashPath, "/go-build") {
		cap.CanUpdate = false
		cap.Reason = i18n.T3For(l,
			"当前为临时编译运行实例（go run），请在电脑端手动更新",
			"Running from a temporary build (go run). Please update manually on your computer.",
			"一時ビルド（go run）で実行中のため、PC側で手動更新してください。",
		)
		return cap
	}
	if strings.EqualFold(strings.TrimSpace(currentVersion), "dev") {
		cap.CanUpdate = false
		cap.Reason = i18n.T3For(l,
			"当前为开发版构建，请在电脑端手动更新",
			"Running a development build. Please update manually on your computer.",
			"開発ビルドのため、PC側で手動更新してください。",
		)
		return cap
	}
	if err := checkDirWritable(targetDir); err != nil {
		cap.CanUpdate = false
		if runtime.GOOS != "windows" {
			cap.Reason = i18n.Sprintf3For(l,
				"目录 %s 无写入权限，请在电脑端执行 sudo mgy update",
				"Directory %s is not writable. Run `sudo mgy update` on your computer.",
				"ディレクトリ %s に書き込み権限がありません。PCで `sudo mgy update` を実行してください。",
				targetDir,
			)
		} else {
			cap.Reason = i18n.Sprintf3For(l,
				"目录 %s 无写入权限，请以管理员身份执行 mgy update",
				"Directory %s is not writable. Run `mgy update` as Administrator.",
				"ディレクトリ %s に書き込み権限がありません。管理者権限で `mgy update` を実行してください。",
				targetDir,
			)
		}
		return cap
	}
	return cap
}

func (o Options) emitProgress(stage, detail string) {
	if o.OnProgress != nil {
		o.OnProgress(stage, detail)
	}
}

// Run checks for updates and optionally downloads and atomically replaces the mgy binary.
func Run(ctx context.Context, opts Options) (*Result, error) {
	o := opts.withDefaults()
	l := i18n.ResolveLang(o.Lang)
	curVer := NormalizeVersion(o.CurrentVersion)

	var tag string
	var err error
	if strings.TrimSpace(o.TargetVersion) != "" {
		tag, err = EnsureTag(o.TargetVersion)
		if err != nil {
			return nil, fmt.Errorf(i18n.T3For(l,
				"版本号格式无效: %w",
				"invalid version format: %w",
				"バージョン形式が無効です: %w",
			), err)
		}
	} else {
		o.emitProgress("checking", i18n.T3For(l,
			"正在检查 mgy 最新版本...",
			"Checking for latest mgy version...",
			"mgy の最新バージョンを確認中...",
		))
		fmt.Fprintf(o.Out, i18n.T3For(l,
			"🔍 正在检查 Multigravity 最新版本 (当前: v%s)...\n",
			"🔍 Checking for latest Multigravity version (current: v%s)...\n",
			"🔍 Multigravity の最新バージョンを確認中 (現在: v%s)...\n",
		), curVer)
		tag, err = FetchLatestTag(ctx, o)
		if err != nil {
			return nil, err
		}
	}

	latestVer := NormalizeVersion(tag)
	res := &Result{
		CurrentVersion: curVer,
		LatestVersion:  latestVer,
		Tag:            tag,
	}

	if strings.TrimSpace(o.TargetVersion) == "" {
		res.UpToDate = !IsNewer(tag, curVer)
	} else {
		res.UpToDate = CompareVersions(tag, curVer) == 0
	}

	if o.CheckOnly {
		return res, nil
	}
	if res.UpToDate && !o.Force {
		return res, nil
	}

	targetPath, err := ResolveExecutablePath(o.TargetPath)
	if err != nil {
		return nil, err
	}
	res.TargetPath = targetPath
	targetDir := filepath.Dir(targetPath)

	// Fail fast before downloading if the install directory is not writable.
	if err := checkDirWritable(targetDir); err != nil {
		if o.GOOS != "windows" {
			return nil, fmt.Errorf(i18n.T3For(l,
				"目标目录 %s 无写入权限 (%v)，请检查权限或使用 sudo mgy update 重试",
				"target directory %s is not writable (%v); check permissions or retry with sudo mgy update",
				"対象ディレクトリ %s に書き込み権限がありません (%v)。権限を確認するか sudo mgy update を試してください",
			), targetDir, err)
		}
		return nil, fmt.Errorf(i18n.T3For(l,
			"目标目录 %s 无写入权限: %w",
			"target directory %s is not writable: %w",
			"対象ディレクトリ %s に書き込み権限がありません: %w",
		), targetDir, err)
	}

	// Best-effort cleanup of leftover backup from a previous Windows self-update.
	_ = os.Remove(targetPath + ".old")

	assets, err := assetCandidatesForLang(o.GOOS, o.GOARCH, l)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(o.TargetVersion) != "" {
		fmt.Fprintf(o.Out, i18n.T3For(l,
			"📦 准备安装目标版本: %s (当前: v%s)\n",
			"📦 Preparing target version: %s (current: v%s)\n",
			"📦 対象バージョンをインストール準備中: %s (現在: v%s)\n",
		), tag, curVer)
	} else if o.Force && res.UpToDate {
		fmt.Fprintf(o.Out, i18n.T3For(l,
			"🔄 强制重新安装当前版本: %s\n",
			"🔄 Reinstalling current version: %s\n",
			"🔄 現在のバージョンを強制再インストール: %s\n",
		), tag)
	} else {
		fmt.Fprintf(o.Out, i18n.T3For(l,
			"🆕 发现新版本: %s (当前: v%s)\n",
			"🆕 New version available: %s (current: v%s)\n",
			"🆕 新しいバージョンが見つかりました: %s (現在: v%s)\n",
		), tag, curVer)
	}

	checksums, hasChecksums, checksumErr := FetchChecksums(ctx, tag, o)
	if checksumErr != nil || !hasChecksums || len(checksums) == 0 {
		return nil, fmt.Errorf(i18n.T3For(l,
			"安全校验失败: 无法从 GitHub 官方 HTTPS 源获取版本 %s 的 checksums.txt (%v)，已中止未校验更新",
			"security verification failed: unable to fetch official checksums.txt for %s (%v); unverified update aborted",
			"セキュリティ検証に失敗: GitHub 公式 HTTPS から %s の checksums.txt を取得できませんでした (%v)。更新を中止しました",
		), tag, checksumErr)
	}

	tmpDir, err := os.MkdirTemp("", "mgy-update-*")
	if err != nil {
		return nil, fmt.Errorf(i18n.T3For(l,
			"创建临时目录失败: %w",
			"failed to create temp directory: %w",
			"一時ディレクトリの作成に失敗しました: %w",
		), err)
	}
	defer os.RemoveAll(tmpDir)

	var lastErr error
	for _, asset := range assets {
		expected, ok := checksums[asset]
		if !ok || expected == "" {
			lastErr = fmt.Errorf(i18n.T3For(l,
				"官方 checksums.txt 中缺少资产 %s 的 SHA-256 记录",
				"official checksums.txt is missing SHA-256 entry for %s",
				"公式 checksums.txt に %s の SHA-256 エントリがありません",
			), asset)
			continue
		}

		ghURL := fmt.Sprintf("%s/%s/releases/download/%s/%s", o.GitHubBaseURL, o.Repo, tag, asset)
		urls := buildDownloadURLs(ghURL, o.MirrorPrefixes)

		for _, rawURL := range urls {
			if o.HTTPClientFactory == nil && !strings.HasPrefix(strings.ToLower(rawURL), "https://") {
				lastErr = fmt.Errorf(i18n.T3For(l,
					"拒绝非 HTTPS 下载源: %s",
					"refusing non-HTTPS download source: %s",
					"非 HTTPS のダウンロード元を拒否しました: %s",
				), rawURL)
				continue
			}
			sourceDesc := i18n.T3For(l, "加速镜像站", "mirror", "ミラー")
			if rawURL == ghURL {
				sourceDesc = i18n.T3For(l, "GitHub 官方源", "GitHub", "GitHub 公式")
			}
			fmt.Fprintf(o.Out, i18n.T3For(l,
				"📥 尝试从%s下载 %s...\n",
				"📥 Downloading %s from %s...\n",
				"📥 %s から %s をダウンロード中...\n",
			), assetOrSourceFirst(l, sourceDesc, asset), assetOrSourceSecond(l, sourceDesc, asset))
			o.emitProgress("downloading", i18n.Sprintf3For(l,
				"正在从%s下载 %s...",
				"Downloading %s from %s...",
				"%s から %s をダウンロード中...",
				assetOrSourceFirst(l, sourceDesc, tag), assetOrSourceSecond(l, sourceDesc, tag),
			))

			archivePath := filepath.Join(tmpDir, asset)
			_ = os.Remove(archivePath)

			gotHash, err := downloadFileWithSHA256(ctx, o, rawURL, archivePath)
			if err != nil {
				fmt.Fprintf(o.Out, i18n.T3For(l,
					"⚠️  下载失败 (%v)，自动切换备用源...\n",
					"⚠️  Download failed (%v), trying next source...\n",
					"⚠️  ダウンロード失敗 (%v)、次のソースへ切り替えます...\n",
				), err)
				lastErr = err
				continue
			}

			o.emitProgress("verifying", i18n.T3For(l,
				"正在校验 SHA-256 与可执行文件...",
				"Verifying SHA-256 and binary...",
				"SHA-256 とバイナリを検証中...",
			))
			if !strings.EqualFold(gotHash, expected) {
				_ = os.Remove(archivePath)
				lastErr = fmt.Errorf(i18n.T3For(l,
					"SHA-256 校验不匹配 (expected %s, got %s)",
					"SHA-256 mismatch (expected %s, got %s)",
					"SHA-256 不一致 (expected %s, got %s)",
				), expected, gotHash)
				fmt.Fprintf(o.Out, i18n.T3For(l,
					"⚠️  %v，丢弃该文件并切换备用源...\n",
					"⚠️  %v, discarding file and trying next source...\n",
					"⚠️  %v、ファイルを破棄して次のソースを試します...\n",
				), lastErr)
				continue
			}
			checksumVerified := true
			checksumSkipped := false
			fmt.Fprintf(o.Out, i18n.T3For(l,
				"🔒 SHA-256 完整性校验通过 (%s...)\n",
				"🔒 SHA-256 verified (%s...)\n",
				"🔒 SHA-256 検証完了 (%s...)\n",
			), gotHash[:12])

			stagedFile, err := os.CreateTemp(targetDir, ".mgy-update-*.tmp")
			if err != nil {
				return nil, fmt.Errorf(i18n.T3For(l,
					"创建暂存文件失败: %w",
					"failed to create staging file: %w",
					"一時ファイルの作成に失敗しました: %w",
				), err)
			}
			stagedPath := stagedFile.Name()
			_ = stagedFile.Close()
			if o.GOOS == "windows" {
				// Windows requires .exe extension to execute for self-verification.
				exeStagedPath := stagedPath + ".exe"
				if rErr := os.Rename(stagedPath, exeStagedPath); rErr == nil {
					stagedPath = exeStagedPath
				}
			}

			if err := extractBinary(archivePath, stagedPath, o.GOOS); err != nil {
				_ = os.Remove(stagedPath)
				fmt.Fprintf(o.Out, i18n.T3For(l,
					"⚠️  归档解压失败 (%v)，切换备用源...\n",
					"⚠️  Archive extraction failed (%v), trying next source...\n",
					"⚠️  アーカイブ展開に失敗 (%v)、次のソースを試します...\n",
				), err)
				lastErr = err
				continue
			}

			if o.GOOS == "darwin" && !o.SkipDarwinSign {
				prepareDarwinBinary(stagedPath)
			}

			if !o.SkipVerifyExec {
				if err := verifyBinaryExecutable(ctx, stagedPath); err != nil {
					_ = os.Remove(stagedPath)
					fmt.Fprintf(o.Out, i18n.T3For(l,
						"⚠️  新版本二进制自检失败 (%v)，切换备用源...\n",
						"⚠️  Binary self-check failed (%v), trying next source...\n",
						"⚠️  バイナリ自己診断に失敗 (%v)、次のソースを試します...\n",
					), err)
					lastErr = err
					continue
				}
			}

			o.emitProgress("replacing", i18n.T3For(l,
				"正在替换网关程序...",
				"Replacing gateway binary...",
				"ゲートウェイバイナリを置換中...",
			))
			if err := replaceBinary(stagedPath, targetPath, o.GOOS); err != nil {
				_ = os.Remove(stagedPath)
				return nil, fmt.Errorf(i18n.T3For(l,
					"替换可执行文件失败: %w",
					"failed to replace binary: %w",
					"実行ファイルの置換に失敗しました: %w",
				), err)
			}

			res.Updated = true
			res.AssetName = asset
			res.DownloadURL = rawURL
			res.ChecksumVerified = checksumVerified
			res.ChecksumSkipped = checksumSkipped
			return res, nil
		}
	}

	return nil, fmt.Errorf(i18n.T3For(l,
		"所有下载源均失败 (%v)\n请检查网络连接或手动前往下载: %s/%s/releases/tag/%s",
		"all download sources failed (%v)\nCheck network connectivity or download manually: %s/%s/releases/tag/%s",
		"すべてのダウンロード元が失敗しました (%v)\nネットワークを確認するか手動でダウンロードしてください: %s/%s/releases/tag/%s",
	), lastErr, o.GitHubBaseURL, o.Repo, tag)
}

func assetOrSourceFirst(lang, sourceDesc, item string) string {
	if i18n.ResolveLang(lang) == i18n.LangEn {
		return item
	}
	return sourceDesc
}

func assetOrSourceSecond(lang, sourceDesc, item string) string {
	if i18n.ResolveLang(lang) == i18n.LangEn {
		return sourceDesc
	}
	return item
}

func buildDownloadURLs(ghURL string, mirrors []string) []string {
	urls := make([]string, 0, len(mirrors)+1)
	for _, prefix := range mirrors {
		if strings.TrimSpace(prefix) != "" {
			urls = append(urls, prefix+ghURL)
		}
	}
	urls = append(urls, ghURL)
	return urls
}

func downloadFileWithSHA256(ctx context.Context, o Options, rawURL, destPath string) (string, error) {
	client := o.newHTTPClient(rawURL, 5*time.Minute)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return "", err
	}
	defer out.Close()

	hasher := sha256.New()
	limited := io.LimitReader(resp.Body, maxArchiveSizeBytes+1)
	n, err := io.Copy(io.MultiWriter(out, hasher), limited)
	if err != nil {
		return "", err
	}
	if n > maxArchiveSizeBytes {
		return "", fmt.Errorf("archive exceeds safety size limit (%d bytes)", maxArchiveSizeBytes)
	}
	if n == 0 {
		return "", fmt.Errorf("downloaded archive is empty")
	}
	if err := out.Sync(); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func extractBinary(archivePath, destPath, goos string) error {
	if strings.HasSuffix(strings.ToLower(archivePath), ".zip") {
		return extractBinaryFromZip(archivePath, destPath, goos)
	}
	return extractBinaryFromTarGz(archivePath, destPath, goos)
}

func isCandidateBinaryName(name, goos string) bool {
	base := filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if goos == "windows" {
		return strings.EqualFold(base, "mgy.exe") || strings.EqualFold(base, "gateway.exe")
	}
	return base == "mgy" || base == "gateway"
}

func extractBinaryFromTarGz(archivePath, destPath, goos string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
			continue
		}
		if !isCandidateBinaryName(hdr.Name, goos) {
			continue
		}
		return writeStagedBinary(destPath, tr)
	}
	return fmt.Errorf("%s", i18n.T3(
		"归档中未找到 mgy 可执行文件",
		"mgy binary not found in archive",
		"アーカイブ内に mgy バイナリが見つかりません",
	))
}

func extractBinaryFromZip(archivePath, destPath, goos string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("zip reader: %w", err)
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if !isCandidateBinaryName(f.Name, goos) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		err = writeStagedBinary(destPath, rc)
		_ = rc.Close()
		return err
	}
	return fmt.Errorf("%s", i18n.T3(
		"zip 归档中未找到 mgy.exe 可执行文件",
		"mgy.exe binary not found in zip archive",
		"zip アーカイブ内に mgy.exe が見つかりません",
	))
}

func writeStagedBinary(destPath string, src io.Reader) error {
	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	n, err := io.Copy(out, io.LimitReader(src, maxBinarySizeBytes+1))
	if err != nil {
		_ = out.Close()
		return err
	}
	if n > maxBinarySizeBytes {
		_ = out.Close()
		return fmt.Errorf("binary exceeds safety size limit")
	}
	if n == 0 {
		_ = out.Close()
		return fmt.Errorf("extracted binary is empty")
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	_ = os.Chmod(destPath, 0755)
	return nil
}

func prepareDarwinBinary(binPath string) {
	_ = exec.Command("xattr", "-d", "com.apple.quarantine", binPath).Run()
	_ = exec.Command("codesign", "-s", "-", "-f", binPath).Run()
}

func verifyBinaryExecutable(ctx context.Context, binPath string) error {
	vctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := procutil.HideWindow(exec.CommandContext(vctx, binPath, "version")).CombinedOutput()
	if err != nil {
		return fmt.Errorf(i18n.T3(
			"执行新版本 `mgy version` 失败: %w (%s)",
			"failed to execute new `mgy version`: %w (%s)",
			"新しい `mgy version` の実行に失敗しました: %w (%s)",
		), err, strings.TrimSpace(string(out)))
	}
	if !strings.Contains(string(out), "Multigravity (mgy)") {
		return fmt.Errorf(i18n.T3(
			"新版本输出异常: %q",
			"unexpected output from new binary: %q",
			"新しいバイナリの出力が不正です: %q",
		), strings.TrimSpace(string(out)))
	}
	return nil
}

func replaceBinary(stagedPath, targetPath, goos string) error {
	if goos != "windows" {
		if err := os.Rename(stagedPath, targetPath); err != nil {
			return err
		}
		_ = os.Chmod(targetPath, 0755)
		return nil
	}

	// Windows: a running .exe cannot be overwritten directly, but it can be renamed away.
	oldPath := targetPath + ".old"
	_ = os.Remove(oldPath)

	hadExisting := false
	if _, err := os.Stat(targetPath); err == nil {
		hadExisting = true
		if err := os.Rename(targetPath, oldPath); err != nil {
			return fmt.Errorf(i18n.T3(
				"无法备份旧版可执行文件: %w",
				"failed to back up previous binary: %w",
				"旧バイナリのバックアップに失敗しました: %w",
			), err)
		}
	}

	if err := os.Rename(stagedPath, targetPath); err != nil {
		if hadExisting {
			_ = os.Rename(oldPath, targetPath)
		}
		return err
	}

	// Best-effort removal of old binary (may still be locked if mgy is running; cleaned up next update).
	_ = os.Remove(oldPath)

	// Keep WindowsApps copy in sync when installed via scripts/install.ps1.
	syncWindowsAppsCopy(targetPath)
	return nil
}

func syncWindowsAppsCopy(targetPath string) {
	localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	if localAppData == "" {
		return
	}
	winAppsCopy := filepath.Join(localAppData, "Microsoft", "WindowsApps", "mgy.exe")
	if strings.EqualFold(filepath.Clean(winAppsCopy), filepath.Clean(targetPath)) {
		return
	}
	if _, err := os.Stat(winAppsCopy); err != nil {
		return
	}
	_ = copyFileBestEffort(targetPath, winAppsCopy)
}

func copyFileBestEffort(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// ProbeRunningGatewayVersion checks whether a local gateway is listening on the given port.
func ProbeRunningGatewayVersion(port int) (string, bool) {
	if port <= 0 {
		return "", false
	}
	client := &http.Client{
		Timeout:   1500 * time.Millisecond,
		Transport: localtls.NewLoopbackTransport(),
	}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", port))
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	var payload struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&payload); err != nil {
		return "", false
	}
	if payload.Status != "ok" {
		return "", false
	}
	return payload.Version, true
}
