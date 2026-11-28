package tunnel

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/procutil"
	"antigravity-mobile/internal/relay"
)

// PinnedCloudflaredVersion is the verified official cloudflared release version.
const PinnedCloudflaredVersion = "2026.11.1"

const maxCloudflaredDownloadBytes = 128 << 20 // 128 MiB safety ceiling (HIGH-02)

// pinnedCloudflaredSHA256 maps official cloudflared release asset filenames for PinnedCloudflaredVersion
// to their authoritative SHA-256 digests published on github.com/cloudflare/cloudflared.
var pinnedCloudflaredSHA256 = map[string]string{
	"cloudflared-darwin-arm64.tgz":  "02fe4bcf08f2c09699cff30658228a739a0c47fb75f3653d1b8772a27ade4462",
	"cloudflared-darwin-amd64.tgz":  "5e0403b188dfee71fbd9b995e7dc9910d61b661c52371935ab6475d41185632f",
	"cloudflared-windows-amd64.exe": "341deda3db5ec498a0482254d6c38c0b7cf01a30c37bb5dd95d2216d08955e35",
	"cloudflared-linux-amd64":       "97bb3b02d23a5980fc9c15a1a270e692da1923f103f920d5b16d4931d5ab55e7",
	"cloudflared-linux-arm64":       "b155cec3e48e489cfb8e5c602769fb8c771083018d8881d68b4d18a791567c5a",
}

// CFTunnelResult contains the acquired public tunnel credentials and endpoint.
// Forward-compatible fields (Mode, Region, RelayEndpoint, ZoneDomain) allow the dispatcher
// to route traffic across multi-zone Cloudflare pools or future embedded relay nodes.
type CFTunnelResult struct {
	Success       bool               `json:"success"`
	Reused        bool               `json:"reused"`
	Mode          string             `json:"mode,omitempty"`
	Region        string             `json:"region,omitempty"`
	RelayEndpoint string             `json:"relay_endpoint,omitempty"`
	ZoneDomain    string             `json:"zone_domain,omitempty"`
	TunnelID      string             `json:"tunnel_id"`
	TunnelName    string             `json:"tunnel_name"`
	Subdomain     string             `json:"subdomain"`
	URL           string             `json:"url"`
	Token         string             `json:"token"`
	RelayNodes    []relay.NodeConfig `json:"relay_nodes,omitempty"`
}

// CloudflareTunnel manages the local cloudflared daemon child process.
type CloudflareTunnel struct {
	result   *CFTunnelResult
	cfg      *config.CloudflareConfig
	cmd      *exec.Cmd
	cmdDone  chan struct{}
	mu       sync.Mutex
	running  bool
	stopping bool
	cancel   context.CancelFunc

	locMu         sync.Mutex
	locations     map[string]string // connIndex -> edge colo, e.g. "0" -> "lax13"
	lastKickAt    time.Time
	lastRefreshAt time.Time
	needsRefresh  bool
	probeResult   EdgeProbe
}

// NewCloudflareTunnel creates a supervisor for a Cloudflare Tunnel.
func NewCloudflareTunnel(res *CFTunnelResult, cfCfg ...*config.CloudflareConfig) *CloudflareTunnel {
	var cfgCopy *config.CloudflareConfig
	if len(cfCfg) > 0 && cfCfg[0] != nil {
		cp := *cfCfg[0]
		cfgCopy = &cp
	}
	return &CloudflareTunnel{
		result: res,
		cfg:    cfgCopy,
	}
}

// PublicURL returns the HTTPS public URL.
func (t *CloudflareTunnel) PublicURL() string {
	if t.result == nil {
		return ""
	}
	return t.result.URL
}

// Subdomain returns the allocated subdomain hostname.
func (t *CloudflareTunnel) Subdomain() string {
	if t.result == nil {
		return ""
	}
	return t.result.Subdomain
}

// Mode returns the tunnel transport mode identifier.
func (t *CloudflareTunnel) Mode() string {
	return ModeCloudflare
}

// Start launches `cloudflared tunnel run --token ...` in the background with auto-recovery supervision.
func (t *CloudflareTunnel) Start(ctx context.Context, binPath string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.running {
		return nil
	}
	if t.result == nil || t.result.Token == "" {
		return fmt.Errorf("missing cloudflare tunnel token")
	}

	tunnelCtx, cancel := context.WithCancel(ctx)
	t.cancel = cancel
	t.stopping = false

	if err := t.launchProcessLocked(tunnelCtx, binPath); err != nil {
		cancel()
		return err
	}

	go t.supervise(tunnelCtx, binPath)
	return nil
}

func (t *CloudflareTunnel) launchProcessLocked(ctx context.Context, binPath string) error {
	cfg := t.cfg
	if cfg != nil && cfg.EdgeProbe && cfg.EdgeIPVersion == "auto" {
		p := ProbeEdge(ctx, nil, nil)
		t.locMu.Lock()
		t.probeResult = p
		t.locMu.Unlock()
		slog.Info(fmt.Sprintf("🔎 [Cloudflare] 边缘探测: IPv4=%s IPv6=%s -> 选用 %s", fmtRTT(p.V4RTT), fmtRTT(p.V6RTT), p.Chosen))
		if len(p.Polluted) > 0 {
			slog.Warn(fmt.Sprintf("⚠️  [Cloudflare] 本机 DNS 解析 argotunnel.com 返回了非 Cloudflare 地址 %v (疑似 DNS 污染)，已自动规避；可设置 CF_DNS_RESOLVERS 指定可信 DNS", p.Polluted))
		}
		c := *cfg
		c.EdgeIPVersion = p.Chosen
		cfg = &c
	}
	t.locMu.Lock()
	t.locations = map[string]string{}
	t.locMu.Unlock()
	args := buildTunnelArgs(cfg)

	cmd := procutil.HideWindow(exec.CommandContext(ctx, binPath, args...))
	// SEC-AUDIT H-3: Pass TUNNEL_TOKEN via environment variable instead of CLI argument
	// to prevent leaking the token in process listings (e.g. ps aux / tasklist).
	cmd.Env = append(os.Environ(), "TUNNEL_TOKEN="+t.result.Token)
	if t.cfg != nil && t.cfg.DNSResolvers != "" && !isDNSOverrideDisabled(t.cfg.DNSResolvers) {
		cmd.Env = append(cmd.Env, "TUNNEL_DNS_RESOLVER_ADDRS="+t.cfg.DNSResolvers)
	}

	// Do not attach stdin. Divert stderr to logger with prefix
	stderr, err := cmd.StderrPipe()
	if err == nil {
		go func() {
			var readyOnce sync.Once
			reader := bufio.NewReader(stderr)
			for {
				line, rErr := reader.ReadString('\n')
				trimmed := strings.TrimSpace(line)
				if trimmed != "" {
					if strings.Contains(trimmed, "Registered tunnel connection") {
						readyOnce.Do(func() {
							slog.Info("✅ [Cloudflare] 专属隧道连接就绪")
						})
						if idx, colo, ok := parseRegisteredLine(trimmed); ok {
							t.recordLocation(idx, colo)
						}
					} else if ip, ok := parseFailedEdgeIP(trimmed); ok && !IsCloudflareEdgeIP(ip) && !IsProxyFakeIP(ip) {
						t.kickPolluted(cmd, ip)
					} else if strings.Contains(trimmed, "TLS handshake with edge error") && strings.Contains(trimmed, "EOF") {
						t.fallbackIPv6ToIPv4(cmd)
					} else if isRevokedOrDeletedTunnelLog(trimmed) {
						t.markStaleCredentials(cmd, trimmed)
					} else if strings.Contains(trimmed, "ERR") || strings.Contains(trimmed, "error") ||
						strings.Contains(trimmed, "Incorrect Usage") || strings.Contains(trimmed, "flag provided") {
						if !isBenignCloudflareLog(trimmed) {
							slog.Warn(fmt.Sprintf("⚠️  [Cloudflare] %s", trimmed))
						}
					}
				}
				if rErr != nil {
					return
				}
			}
		}()
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start cloudflared: %w", err)
	}

	t.cmd = cmd
	t.cmdDone = make(chan struct{})
	t.running = true
	return nil
}

func fmtRTT(d time.Duration) string {
	if d <= 0 {
		return "不可达"
	}
	return fmt.Sprintf("%dms", d.Milliseconds())
}

// recordLocation silently remembers which edge colo a tunnel connection landed on
// (exposed via Locations for diagnostics; intentionally not printed to the console).
func (t *CloudflareTunnel) recordLocation(idx, colo string) {
	t.locMu.Lock()
	defer t.locMu.Unlock()
	if t.locations == nil {
		t.locations = map[string]string{}
	}
	t.locations[idx] = colo
}

// Locations returns the distinct edge colos currently serving this tunnel, sorted.
func (t *CloudflareTunnel) Locations() []string {
	t.locMu.Lock()
	defer t.locMu.Unlock()
	seen := map[string]bool{}
	var out []string
	for _, c := range t.locations {
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out
}

// kickPolluted restarts cloudflared (at most once per 5 minutes) after it tried to dial a
// non-Cloudflare edge address, so the supervisor re-probes and pins a clean IP family.
func (t *CloudflareTunnel) kickPolluted(cmd *exec.Cmd, ip net.IP) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopping || t.cmd != cmd || cmd.Process == nil || time.Since(t.lastKickAt) < 5*time.Minute {
		return
	}
	t.lastKickAt = time.Now()
	slog.Warn(fmt.Sprintf("⚠️  [Cloudflare] 检测到边缘地址被污染 (%s)，重启隧道并重新选择可用线路", ip))
	_ = cmd.Process.Kill()
}

// fallbackIPv6ToIPv4 falls back to IPv4 when IPv6 edge TLS handshakes persistently fail (e.g. EOF caused by TUN proxy / ISP resets).
func (t *CloudflareTunnel) fallbackIPv6ToIPv4(cmd *exec.Cmd) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopping || t.cmd != cmd || cmd.Process == nil || time.Since(t.lastKickAt) < 2*time.Minute {
		return
	}
	if t.cfg != nil && t.cfg.EdgeIPVersion != "4" {
		t.lastKickAt = time.Now()
		t.cfg.EdgeIPVersion = "4"
		slog.Warn("⚠️  [Cloudflare] 边缘节点 IPv6 TLS 握手频繁失败 (EOF)，自动切换并锁定 IPv4 专线连接...")
		_ = cmd.Process.Kill()
	}
}

// markStaleCredentials invalidates cached tunnel credentials when Cloudflare reports a GC'd or revoked tunnel,
// allowing supervise() to seamlessly re-provision the deterministic machine tunnel via the dispatcher Worker.
func (t *CloudflareTunnel) markStaleCredentials(cmd *exec.Cmd, reason string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopping || t.cfg == nil || t.cfg.Token != "" || strings.TrimSpace(t.cfg.WorkerURL) == "" {
		return
	}
	if time.Since(t.lastRefreshAt) < time.Minute {
		return
	}
	t.lastRefreshAt = time.Now()
	t.needsRefresh = true
	_ = InvalidateTunnelCache()
	slog.Warn(fmt.Sprintf("⚠️  [Cloudflare] 本地缓存的穿透凭据已失效或被回收 (%s)，正在自动重新申请专属隧道...", reason))
	if t.cmd == cmd && cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

var tunnelCacheMu sync.Mutex

// InvalidateTunnelCache removes the locally cached cf_tunnel.json file.
func InvalidateTunnelCache() error {
	tunnelCacheMu.Lock()
	defer tunnelCacheMu.Unlock()
	cachePath := filepath.Join(config.GetDataDir(), "cf_tunnel.json")
	if err := os.Remove(cachePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func writeTunnelCacheLocked(cachePath string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(cachePath), 0700); err != nil {
		return err
	}
	tmpPath := cachePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, cachePath); err != nil {
		_ = os.Remove(tmpPath)
		return os.WriteFile(cachePath, data, 0600)
	}
	return nil
}

const (
	minReconnectBackoff  = 3 * time.Second
	maxReconnectBackoff  = 60 * time.Second
	healthyRunResetAfter = 30 * time.Second
)

// nextReconnectBackoff computes exponential backoff (3s -> 6s -> 12s -> 24s -> 48s -> max 60s)
// with ±20% jitter so offline networks neither busy-spin every 3s nor thunder-herd on recovery (P1-5).
func nextReconnectBackoff(failures int) time.Duration {
	if failures < 0 {
		failures = 0
	}
	if failures > 5 {
		failures = 5
	}
	base := minReconnectBackoff << failures
	if base > maxReconnectBackoff {
		base = maxReconnectBackoff
	}
	var jb [1]byte
	if _, err := rand.Read(jb[:]); err == nil {
		// Map 0..255 to -20%..+20%
		delta := (int64(jb[0]) - 128) * int64(base) / 640
		base = time.Duration(int64(base) + delta)
	}
	if base < time.Second {
		base = time.Second
	}
	return base
}

func (t *CloudflareTunnel) supervise(ctx context.Context, binPath string) {
	failures := 0
	for {
		t.mu.Lock()
		cmd := t.cmd
		doneCh := t.cmdDone
		t.mu.Unlock()

		startedAt := time.Now()
		if cmd != nil {
			_ = cmd.Wait()
			if doneCh != nil {
				close(doneCh)
			}
		}
		if time.Since(startedAt) >= healthyRunResetAfter {
			failures = 0
		}

		t.mu.Lock()
		t.running = false
		if t.stopping || ctx.Err() != nil {
			t.mu.Unlock()
			return
		}
		shouldRefresh := t.needsRefresh && t.cfg != nil && t.cfg.Token == "" && strings.TrimSpace(t.cfg.WorkerURL) != ""
		var workerURL, inviteCode string
		if shouldRefresh {
			workerURL = t.cfg.WorkerURL
			inviteCode = t.cfg.InviteCode
		}
		t.mu.Unlock()

		delay := nextReconnectBackoff(failures)
		failures++

		if shouldRefresh {
			_ = InvalidateTunnelCache()
			if refreshed, err := RegisterOrFetchTunnel(ctx, workerURL, inviteCode); err == nil && refreshed != nil && refreshed.Token != "" {
				t.mu.Lock()
				t.result = refreshed
				t.needsRefresh = false
				t.mu.Unlock()
				failures = 0
				delay = minReconnectBackoff
				slog.Info(fmt.Sprintf("✅ [Cloudflare] 专属隧道凭据已自动刷新: %s", refreshed.URL))
			} else {
				slog.Warn("⚠️  [Cloudflare] 重新申请专属隧道凭据失败，稍后退避重试", "err", err, "retry_in", delay.Round(100*time.Millisecond))
			}
		} else {
			slog.Warn(fmt.Sprintf("⚠️  [Cloudflare] 穿透守护进程退出，将在 %s 后尝试自动恢复连接...", delay.Round(100*time.Millisecond)))
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		t.mu.Lock()
		if t.stopping || ctx.Err() != nil {
			t.mu.Unlock()
			return
		}
		if err := t.launchProcessLocked(ctx, binPath); err != nil {
			slog.Warn("⚠️  [Cloudflare] 自动恢复穿透失败", "err", err)
		}
		t.mu.Unlock()
	}
}

// Stop terminates the running cloudflared process gracefully with fallback to Kill.
func (t *CloudflareTunnel) Stop() {
	t.mu.Lock()
	t.stopping = true
	if t.cancel != nil {
		t.cancel()
	}
	if !t.running || t.cmd == nil || t.cmd.Process == nil {
		t.mu.Unlock()
		return
	}
	proc := t.cmd.Process
	doneCh := t.cmdDone
	t.running = false
	t.mu.Unlock()

	_ = proc.Signal(os.Interrupt)
	go func() {
		if doneCh != nil {
			timer := time.NewTimer(2500 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-doneCh:
				return
			case <-timer.C:
			}
		} else {
			time.Sleep(2500 * time.Millisecond)
		}
		_ = proc.Kill()
	}()
}

// isValidCloudflared checks whether a cloudflared executable exists, is >= 5MB, and can execute `--version`.
func isValidCloudflared(binPath string) bool {
	if binPath == "" {
		return false
	}
	fi, err := os.Stat(binPath)
	if err != nil || fi.IsDir() || fi.Size() < 5*1024*1024 {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := procutil.HideWindow(exec.CommandContext(ctx, binPath, "--version"))
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

// FindCloudflaredBinary checks whether cloudflared exists in PATH or ~/.multigravity/bin.
func FindCloudflaredBinary() string {
	if bin, err := exec.LookPath("cloudflared"); err == nil {
		if isValidCloudflared(bin) {
			return bin
		}
		slog.Warn(fmt.Sprintf("⚠️  系统路径中的 cloudflared 损坏或无效 (%s)，将尝试使用本地专用版本...", bin))
	}
	binDir := filepath.Join(config.GetDataDir(), "bin")
	binName := "cloudflared"
	if runtime.GOOS == "windows" {
		binName = "cloudflared.exe"
	}
	targetPath := filepath.Join(binDir, binName)
	if isValidCloudflared(targetPath) {
		return targetPath
	}
	return ""
}

// EnsureCloudflaredBinary checks for cloudflared in PATH or downloads it automatically.
func EnsureCloudflaredBinary(ctx context.Context) (string, error) {
	if bin := FindCloudflaredBinary(); bin != "" {
		return bin, nil
	}

	binDir := filepath.Join(config.GetDataDir(), "bin")
	binName := "cloudflared"
	if runtime.GOOS == "windows" {
		binName = "cloudflared.exe"
	}
	targetPath := filepath.Join(binDir, binName)


	// 3. Needs download
	_ = os.MkdirAll(binDir, 0755)
	slog.Info("⏬ 正在拉取 Cloudflare 穿透引擎二进制文件 (~65MB)...")

	downloadURLs := getCloudflaredDownloadURLs()
	if len(downloadURLs) == 0 {
		return "", fmt.Errorf("unsupported platform for auto-download: %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	var downloadErr error
	for _, rawURL := range downloadURLs {
		downloadErr = downloadAndInstallBinary(ctx, rawURL, targetPath)
		if downloadErr == nil {
			slog.Info(fmt.Sprintf("✅ Cloudflare 穿透引擎安装成功: %s", targetPath))
			return targetPath, nil
		}
		slog.Warn(fmt.Sprintf("⚠️  下载源 %s 失败: %v, 尝试备用源...", rawURL, downloadErr))
	}

	return "", fmt.Errorf("all download mirrors failed: %w", downloadErr)
}

func getCloudflaredDownloadURLs() []string {
	var baseNames []string
	switch runtime.GOOS {
	case "darwin":
		if runtime.GOARCH == "arm64" {
			baseNames = []string{"cloudflared-darwin-arm64.tgz"}
		} else {
			baseNames = []string{"cloudflared-darwin-amd64.tgz"}
		}
	case "windows":
		baseNames = []string{"cloudflared-windows-amd64.exe"}
	case "linux":
		if runtime.GOARCH == "arm64" {
			baseNames = []string{"cloudflared-linux-arm64"}
		} else {
			baseNames = []string{"cloudflared-linux-amd64"}
		}
	default:
		return nil
	}

	var urls []string
	for _, fn := range baseNames {
		ghURL := fmt.Sprintf("https://github.com/cloudflare/cloudflared/releases/download/%s/%s", PinnedCloudflaredVersion, fn)
		// 1. 优先使用国内知名加速镜像源（下载后强制校验官方 SHA-256 白名单，防篡改）
		urls = append(urls, "https://ghfast.top/"+ghURL)
		urls = append(urls, "https://ghproxy.net/"+ghURL)
		// 2. 官方直链兜底
		urls = append(urls, ghURL)
	}
	return urls
}

// detectLocalProxy checks environment variables and local proxy ports (with 60s caching).
func detectLocalProxy() *url.URL {
	return config.DetectLocalProxy()
}

func createDownloadHTTPClient(downloadURL string) *http.Client {
	isOfficialGitHub := strings.HasPrefix(downloadURL, "https://github.com/")

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	}

	if isOfficialGitHub {
		// 官方源：尝试使用环境变量或本地探测到的代理
		if proxyURL := detectLocalProxy(); proxyURL != nil {
			slog.Info(fmt.Sprintf("⚡ 官方 GitHub 源将使用代理加速连接: %s", proxyURL.String()))
			transport.Proxy = http.ProxyURL(proxyURL)
		} else {
			transport.Proxy = http.ProxyFromEnvironment
		}
	} else {
		// 加速镜像站：强制直连，不走代理（避免代理节点干扰或限流）
		transport.Proxy = nil
	}

	// 总体超时放宽至 5 分钟，支持大文件在慢速网络下平稳下载
	return &http.Client{
		Transport: transport,
		Timeout:   5 * time.Minute,
	}
}

func expectedCloudflaredSHA256(downloadURL string) (string, error) {
	parsed, err := url.Parse(downloadURL)
	if err != nil {
		return "", fmt.Errorf("invalid download URL: %w", err)
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return "", fmt.Errorf("refusing non-HTTPS cloudflared URL: %s", downloadURL)
	}
	assetName := filepath.Base(parsed.Path)
	sum, ok := pinnedCloudflaredSHA256[assetName]
	if !ok || sum == "" {
		return "", fmt.Errorf("untrusted cloudflared asset %q (no pinned SHA-256)", assetName)
	}
	return sum, nil
}

func downloadAndInstallBinary(ctx context.Context, downloadURL, destPath string) error {
	expectedSHA, err := expectedCloudflaredSHA256(downloadURL)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Multigravity-AutoInstaller/1.0")

	client := createDownloadHTTPClient(downloadURL)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	dlTmp := destPath + ".download.tmp"
	tmpFile := destPath + ".tmp"
	defer os.Remove(dlTmp)
	defer os.Remove(tmpFile)

	// HIGH-02: Stream into non-executable staging file with 128 MiB cap and verify official SHA-256
	// BEFORE extracting or marking executable.
	dlOut, err := os.OpenFile(dlTmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	hasher := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(dlOut, hasher), io.LimitReader(resp.Body, maxCloudflaredDownloadBytes+1))
	closeErr := dlOut.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if n > maxCloudflaredDownloadBytes {
		return fmt.Errorf("cloudflared download exceeds 128 MiB limit")
	}
	if n == 0 {
		return fmt.Errorf("cloudflared download is empty")
	}
	gotSHA := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(gotSHA, expectedSHA) {
		return fmt.Errorf("cloudflared SHA-256 mismatch (expected %s, got %s)", expectedSHA, gotSHA)
	}

	if strings.HasSuffix(downloadURL, ".tgz") || strings.HasSuffix(downloadURL, ".tar.gz") {
		archiveFile, err := os.Open(dlTmp)
		if err != nil {
			return err
		}
		defer archiveFile.Close()

		gzr, err := gzip.NewReader(archiveFile)
		if err != nil {
			return fmt.Errorf("gzip reader: %w", err)
		}
		defer gzr.Close()

		tr := tar.NewReader(gzr)
		found := false
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("tar read: %w", err)
			}
			if (hdr.Typeflag == tar.TypeReg || hdr.Typeflag == tar.TypeRegA) &&
				(hdr.Name == "cloudflared" || strings.HasSuffix(hdr.Name, "/cloudflared")) {
				out, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
				if err != nil {
					return err
				}
				written, err := io.Copy(out, io.LimitReader(tr, maxCloudflaredDownloadBytes+1))
				if err != nil {
					out.Close()
					return err
				}
				if written > maxCloudflaredDownloadBytes || written == 0 {
					out.Close()
					return fmt.Errorf("invalid extracted cloudflared size (%d bytes)", written)
				}
				if err := out.Close(); err != nil {
					return err
				}
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("binary 'cloudflared' not found inside archive")
		}
	} else {
		if err := os.Rename(dlTmp, tmpFile); err != nil {
			return err
		}
	}

	if err := os.Rename(tmpFile, destPath); err != nil {
		return err
	}
	_ = os.Chmod(destPath, 0755)

	if !isValidCloudflared(destPath) {
		_ = os.Remove(destPath)
		return fmt.Errorf("downloaded binary failed validation")
	}

	return nil
}

// GetStableMachineID retrieves an existing persistent machine credential or generates a new
// 256-bit cryptographically random secret on first launch (CRIT-02).
// Existing non-empty machine_id files are reused seamlessly to preserve allocated subdomains.
func GetStableMachineID() string {
	dataDir := config.GetDataDir()
	idFile := filepath.Join(dataDir, "machine_id")
	if data, err := os.ReadFile(idFile); err == nil {
		str := strings.TrimSpace(string(data))
		if len(str) >= 16 {
			_ = os.Chmod(idFile, 0600)
			return str
		}
	}

	raw := make([]byte, 32)
	var rawID string
	if _, err := rand.Read(raw); err == nil {
		rawID = hex.EncodeToString(raw)
	} else {
		// Emergency fallback only if OS CSPRNG fails
		h := sha256.New()
		if name, hErr := os.Hostname(); hErr == nil {
			h.Write([]byte(name))
		}
		fmt.Fprintf(h, ":%d", time.Now().UnixNano())
		rawID = hex.EncodeToString(h.Sum(nil))
	}

	_ = os.MkdirAll(dataDir, 0700)
	tmpFile := idFile + ".tmp"
	if err := os.WriteFile(tmpFile, []byte(rawID+"\n"), 0600); err == nil {
		if rErr := os.Rename(tmpFile, idFile); rErr != nil {
			_ = os.Remove(tmpFile)
			_ = os.WriteFile(idFile, []byte(rawID+"\n"), 0600)
		}
	} else {
		_ = os.WriteFile(idFile, []byte(rawID+"\n"), 0600)
	}
	return rawID
}

// LoadCachedTunnelResult reads locally cached Cloudflare Tunnel credentials without network I/O.
func LoadCachedTunnelResult() *CFTunnelResult {
	tunnelCacheMu.Lock()
	defer tunnelCacheMu.Unlock()
	return loadCachedTunnelResultLocked(filepath.Join(config.GetDataDir(), "cf_tunnel.json"))
}

func loadCachedTunnelResultLocked(cachePath string) *CFTunnelResult {
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil
	}
	var cached CFTunnelResult
	if err := json.Unmarshal(data, &cached); err != nil || cached.Token == "" || cached.URL == "" {
		return nil
	}
	cached.Reused = true
	return &cached
}

// UpdateCachedRelayPool atomically updates cf_tunnel.json with the active connected relay_v1
// node and/or refreshed relay_nodes pool so subsequent startups reuse the fastest active VPS
// instead of printing a stale initial relay_nodes[0] domain.
// Never overwrites an existing non-relay_v1 (Cloudflare) tunnel cache unless base is relay_v1.
func UpdateCachedRelayPool(active *relay.NodeConfig, nodes []relay.NodeConfig, base *CFTunnelResult) error {
	tunnelCacheMu.Lock()
	defer tunnelCacheMu.Unlock()

	cachePath := filepath.Join(config.GetDataDir(), "cf_tunnel.json")
	var res CFTunnelResult
	if base != nil {
		if base.EffectiveMode() != ModeRelayV1 {
			return nil
		}
		res = *base
		if len(base.RelayNodes) > 0 {
			res.RelayNodes = append([]relay.NodeConfig(nil), base.RelayNodes...)
		}
	} else if existing := loadCachedTunnelResultLocked(cachePath); existing != nil {
		if existing.EffectiveMode() != ModeRelayV1 {
			return nil
		}
		res = *existing
	} else {
		res = CFTunnelResult{
			Success: true,
			Mode:    ModeRelayV1,
		}
	}

	if len(nodes) > 0 {
		res.RelayNodes = append([]relay.NodeConfig(nil), nodes...)
	}

	if active != nil {
		u := strings.TrimSpace(active.URL)
		tok := strings.TrimSpace(active.Token)
		ep := strings.TrimSpace(active.Endpoint)
		if u != "" && tok != "" && ep != "" {
			res.Success = true
			res.Mode = ModeRelayV1
			res.URL = u
			res.Token = tok
			res.RelayEndpoint = ep
			sub := strings.TrimSpace(active.Subdomain)
			if sub == "" {
				if parsed, err := url.Parse(u); err == nil {
					sub = strings.TrimSpace(parsed.Hostname())
				}
			}
			res.Subdomain = sub
			if reg := strings.TrimSpace(active.Region); reg != "" {
				res.Region = reg
			}
			if len(res.RelayNodes) == 0 {
				res.RelayNodes = []relay.NodeConfig{*active}
			}
		}
	}

	if res.EffectiveMode() != ModeRelayV1 || strings.TrimSpace(res.Token) == "" || strings.TrimSpace(res.URL) == "" {
		return nil
	}
	res.Reused = false

	encoded, err := json.Marshal(&res)
	if err != nil {
		return err
	}
	return writeTunnelCacheLocked(cachePath, encoded)
}

func hasCapability(caps []string, mode string) bool {
	target := strings.ToLower(strings.TrimSpace(mode))
	if target == "" {
		target = ModeCloudflare
	}
	for _, c := range caps {
		if strings.EqualFold(strings.TrimSpace(c), target) {
			return true
		}
	}
	return false
}

// RegisterOrFetchTunnel requests a permanent tunnel from the Worker or loads from cache.
func RegisterOrFetchTunnel(ctx context.Context, workerURL, inviteCode string) (*CFTunnelResult, error) {
	return RegisterOrFetchTunnelWithOptions(ctx, workerURL, inviteCode, RegisterOptions{
		ClientVersion: GetClientVersion(),
		Capabilities:  SupportedCapabilities(),
	})
}

// RegisterOrFetchTunnelWithOptions requests a permanent tunnel from the dispatcher with explicit client metadata.
func RegisterOrFetchTunnelWithOptions(ctx context.Context, workerURL, inviteCode string, opts RegisterOptions) (*CFTunnelResult, error) {
	cachePath := filepath.Join(config.GetDataDir(), "cf_tunnel.json")
	caps := opts.Capabilities
	if len(caps) == 0 {
		caps = SupportedCapabilities()
	}

	// 1. Try reading from cache (only reuse if its mode is supported by this build)
	if !opts.SkipCache {
		if data, err := os.ReadFile(cachePath); err == nil {
			var cached CFTunnelResult
			if err := json.Unmarshal(data, &cached); err == nil && cached.Token != "" && cached.URL != "" {
				if hasCapability(caps, cached.EffectiveMode()) {
					cached.Reused = true
					return &cached, nil
				}
			}
		}
	}

	cleanWorker := strings.TrimRight(strings.TrimSpace(workerURL), "/")
	if cleanWorker == "" {
		return nil, fmt.Errorf("CF_WORKER_URL is empty")
	}

	machineID := GetStableMachineID()
	reqBody := map[string]any{
		"machine_id":   machineID,
		"platform":     fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH),
		"invite_code":  inviteCode,
		"capabilities": caps,
	}
	if ver := strings.TrimSpace(opts.ClientVersion); ver != "" {
		reqBody["client_version"] = ver
	}
	bodyBytes, _ := json.Marshal(reqBody)

	registerURL := cleanWorker + "/api/tunnel/register"
	req, err := http.NewRequestWithContext(ctx, "POST", registerURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if inviteCode != "" {
		req.Header.Set("X-Invite-Code", inviteCode)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request worker failed: %w", err)
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read worker response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("worker error (HTTP %d): %s", resp.StatusCode, string(respData))
	}

	var result CFTunnelResult
	if err := json.Unmarshal(respData, &result); err != nil {
		return nil, fmt.Errorf("parse worker response: %w", err)
	}

	if len(result.RelayNodes) > 0 {
		first := result.RelayNodes[0]
		if result.Token == "" {
			result.Token = first.Token
		}
		if result.URL == "" {
			result.URL = first.URL
		}
		if result.Subdomain == "" {
			result.Subdomain = first.Subdomain
		}
		if result.RelayEndpoint == "" {
			result.RelayEndpoint = first.Endpoint
		}
	}

	if result.Token == "" || result.URL == "" {
		return nil, fmt.Errorf("invalid worker response, missing token/url: %s", string(respData))
	}
	result.Mode = result.EffectiveMode()

	// 2. Cache result locally (unless SkipCache is set for dual-track or pool refresh calls)
	if !opts.SkipCache {
		tunnelCacheMu.Lock()
		_ = writeTunnelCacheLocked(cachePath, respData)
		tunnelCacheMu.Unlock()
	}
	return &result, nil
}

// buildTunnelArgs constructs the CLI arguments for running cloudflared.
// In cloudflared CLI:
//   - tunnel options (e.g. --edge-ip-version, --region) must appear before 'run'
//   - run options (e.g. --protocol, --dns-resolver-addrs) must appear after 'run'
func buildTunnelArgs(cfg *config.CloudflareConfig) []string {
	args := []string{"tunnel"}
	if cfg != nil {
		if cfg.EdgeIPVersion != "" {
			args = append(args, "--edge-ip-version", cfg.EdgeIPVersion)
		}
		if cfg.Region != "" {
			args = append(args, "--region", cfg.Region)
		}
	}
	args = append(args, "run")
	if cfg != nil {
		if cfg.Protocol != "" {
			args = append(args, "--protocol", cfg.Protocol)
		}
		if cfg.DNSResolvers != "" && !isDNSOverrideDisabled(cfg.DNSResolvers) {
			for _, addr := range strings.Split(cfg.DNSResolvers, ",") {
				addr = strings.TrimSpace(addr)
				if addr != "" {
					if !strings.Contains(addr, ":") {
						addr = addr + ":53"
					}
					args = append(args, "--dns-resolver-addrs", addr)
				}
			}
		}
	}
	return args
}

// isDNSOverrideDisabled reports whether DNS resolver override is explicitly disabled.
func isDNSOverrideDisabled(val string) bool {
	v := strings.ToLower(strings.TrimSpace(val))
	return v == "none" || v == "off" || v == "disable" || v == "disabled" || v == "system" || v == "0"
}

// isBenignCloudflareLog reports whether a log line is an expected non-fatal transient message.
func isBenignCloudflareLog(line string) bool {
	benignKeywords := []string{
		"context canceled",
		"already connected to this server",
		"Failed to initialize DNS local resolver",
		"Failed to refresh DNS local resolver",
		"ping_group_range",
		"ICMP proxy feature is disabled",
	}
	for _, kw := range benignKeywords {
		if strings.Contains(line, kw) {
			return true
		}
	}
	return false
}

// isRevokedOrDeletedTunnelLog reports whether a cloudflared stderr line indicates the tunnel or its token
// was deleted/revoked on Cloudflare (e.g., reclaimed by stale-tunnel GC after prolonged inactivity).
func isRevokedOrDeletedTunnelLog(line string) bool {
	lower := strings.ToLower(line)
	revokedKeywords := []string{
		"tunnel not found",
		"failed to authenticate",
		"failed to sufficiently authenticate",
		"unauthorized: invalid tunnel",
		"invalid tunnel secret",
		"tunnel has been deleted",
	}
	for _, kw := range revokedKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

