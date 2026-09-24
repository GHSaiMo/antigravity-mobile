package tunnel

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/config"
)

// CFTunnelResult contains the acquired Cloudflare Tunnel credentials and endpoint.
type CFTunnelResult struct {
	Success    bool   `json:"success"`
	Reused     bool   `json:"reused"`
	TunnelID   string `json:"tunnel_id"`
	TunnelName string `json:"tunnel_name"`
	Subdomain  string `json:"subdomain"`
	URL        string `json:"url"`
	Token      string `json:"token"`
}

// CloudflareTunnel manages the local cloudflared daemon child process.
type CloudflareTunnel struct {
	result  *CFTunnelResult
	cfg     *config.CloudflareConfig
	cmd     *exec.Cmd
	mu      sync.Mutex
	running bool
}

// NewCloudflareTunnel creates a supervisor for a Cloudflare Tunnel.
func NewCloudflareTunnel(res *CFTunnelResult, cfCfg ...*config.CloudflareConfig) *CloudflareTunnel {
	var cfg *config.CloudflareConfig
	if len(cfCfg) > 0 {
		cfg = cfCfg[0]
	}
	return &CloudflareTunnel{
		result: res,
		cfg:    cfg,
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

// Start launches `cloudflared tunnel run --token ...` in the background.
func (t *CloudflareTunnel) Start(ctx context.Context, binPath string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.running {
		return nil
	}
	if t.result == nil || t.result.Token == "" {
		return fmt.Errorf("missing cloudflare tunnel token")
	}

	args := []string{"tunnel"}
	if t.cfg != nil {
		if t.cfg.EdgeIPVersion != "" {
			args = append(args, "--edge-ip-version", t.cfg.EdgeIPVersion)
		}
		if t.cfg.Region != "" {
			args = append(args, "--region", t.cfg.Region)
		}
		if t.cfg.Protocol != "" {
			args = append(args, "--protocol", t.cfg.Protocol)
		}
	}
	args = append(args, "run")

	cmd := exec.CommandContext(ctx, binPath, args...)
	// SEC-AUDIT H-3: Pass TUNNEL_TOKEN via environment variable instead of CLI argument
	// to prevent leaking the token in process listings (e.g. ps aux / tasklist).
	cmd.Env = append(os.Environ(), "TUNNEL_TOKEN="+t.result.Token)
	// Do not attach stdin. Divert stderr to logger with prefix
	stderr, err := cmd.StderrPipe()
	if err == nil {
		go func() {
			reader := bufio.NewReader(stderr)
			var connectedOnce sync.Once
			for {
				line, rErr := reader.ReadString('\n')
				trimmed := strings.TrimSpace(line)
				if trimmed != "" {
					// Condense multiple tunnel connections into one simple prompt
					if strings.Contains(trimmed, "Registered tunnel connection") ||
						(strings.Contains(trimmed, "Connection") && strings.Contains(trimmed, "registered")) {
						connectedOnce.Do(func() {
							log.Printf("☁️  已与 Cloudflare 专属域名建立连接")
						})
					} else if strings.Contains(trimmed, "ERR") || strings.Contains(trimmed, "error") {
						if !strings.Contains(trimmed, "context canceled") {
							log.Printf("⚠️  [Cloudflare] %s", trimmed)
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
	t.running = true

	go func() {
		_ = cmd.Wait()
		t.mu.Lock()
		t.running = false
		t.mu.Unlock()
	}()

	return nil
}

// Stop terminates the running cloudflared process gracefully with fallback to Kill.
func (t *CloudflareTunnel) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.running || t.cmd == nil || t.cmd.Process == nil {
		return
	}
	proc := t.cmd.Process
	_ = proc.Signal(os.Interrupt)
	t.running = false

	go func() {
		time.Sleep(2500 * time.Millisecond)
		_ = proc.Kill()
	}()
}

// FindCloudflaredBinary checks whether cloudflared exists in PATH or ~/.multigravity/bin.
func FindCloudflaredBinary() string {
	if bin, err := exec.LookPath("cloudflared"); err == nil {
		return bin
	}
	binDir := filepath.Join(config.GetDataDir(), "bin")
	binName := "cloudflared"
	if runtime.GOOS == "windows" {
		binName = "cloudflared.exe"
	}
	targetPath := filepath.Join(binDir, binName)
	if fi, err := os.Stat(targetPath); err == nil && !fi.IsDir() && fi.Size() > 1024*1024 {
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
	log.Printf("⏬ 正在拉取 Cloudflare 穿透引擎二进制文件 (~65MB)...")

	downloadURLs := getCloudflaredDownloadURLs()
	if len(downloadURLs) == 0 {
		return "", fmt.Errorf("unsupported platform for auto-download: %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	var downloadErr error
	for _, rawURL := range downloadURLs {
		downloadErr = downloadAndInstallBinary(ctx, rawURL, targetPath)
		if downloadErr == nil {
			log.Printf("✅ Cloudflare 穿透引擎安装成功: %s", targetPath)
			return targetPath, nil
		}
		log.Printf("⚠️  下载源 %s 失败: %v, 尝试备用源...", rawURL, downloadErr)
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
		ghURL := fmt.Sprintf("https://github.com/cloudflare/cloudflared/releases/latest/download/%s", fn)
		// 1. 优先使用国内知名加速镜像源
		urls = append(urls, "https://ghfast.top/"+ghURL)
		urls = append(urls, "https://ghproxy.net/"+ghURL)
		// 2. 官方直链兜底
		urls = append(urls, ghURL)
	}
	return urls
}

// detectLocalProxy checks environment variables and local proxy ports (Clash/V2Ray/Surge/Sing-box).
func detectLocalProxy() *url.URL {
	for _, envKey := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy"} {
		if val := strings.TrimSpace(os.Getenv(envKey)); val != "" {
			if !strings.Contains(val, "://") {
				val = "http://" + val
			}
			if u, err := url.Parse(val); err == nil {
				return u
			}
		}
	}

	// 自动探测本地常用代理端口
	commonPorts := []int{7890, 10808, 1080, 6152}
	for _, p := range commonPorts {
		addr := fmt.Sprintf("127.0.0.1:%d", p)
		conn, err := net.DialTimeout("tcp", addr, 150*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return &url.URL{
				Scheme: "http",
				Host:   addr,
			}
		}
	}
	return nil
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
			log.Printf("⚡ 官方 GitHub 源将使用代理加速连接: %s", proxyURL.String())
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

func downloadAndInstallBinary(ctx context.Context, downloadURL, destPath string) error {
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

	tmpFile := destPath + ".tmp"
	defer os.Remove(tmpFile)

	if strings.HasSuffix(downloadURL, ".tgz") || strings.HasSuffix(downloadURL, ".tar.gz") {
		// 解压 tar.gz 提取 cloudflared 单文件
		gzr, err := gzip.NewReader(resp.Body)
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
			if !hdr.FileInfo().IsDir() && (hdr.Name == "cloudflared" || strings.HasSuffix(hdr.Name, "/cloudflared")) {
				out, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
				if err != nil {
					return err
				}
				if _, err := io.Copy(out, tr); err != nil {
					out.Close()
					return err
				}
				out.Close()
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("binary 'cloudflared' not found inside archive")
		}
	} else {
		// 直接是二进制文件 (如 Windows .exe)
		out, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, resp.Body); err != nil {
			out.Close()
			return err
		}
		out.Close()
	}

	return os.Rename(tmpFile, destPath)
}

// GetStableMachineID retrieves or creates a persistent unique machine fingerprint.
func GetStableMachineID() string {
	idFile := filepath.Join(config.GetDataDir(), "machine_id")
	if data, err := os.ReadFile(idFile); err == nil {
		str := strings.TrimSpace(string(data))
		if len(str) >= 16 {
			return str
		}
	}

	// Generate deterministic hash from hostname + MAC addresses
	h := sha256.New()
	if name, err := os.Hostname(); err == nil {
		h.Write([]byte(name))
	}
	if ifaces, err := net.Interfaces(); err == nil {
		for _, iface := range ifaces {
			if len(iface.HardwareAddr) > 0 {
				h.Write(iface.HardwareAddr)
			}
		}
	}
	h.Write([]byte(runtime.GOOS + runtime.GOARCH))
	rawID := hex.EncodeToString(h.Sum(nil))

	_ = os.MkdirAll(config.GetDataDir(), 0700)
	_ = os.WriteFile(idFile, []byte(rawID), 0600)
	return rawID
}

// RegisterOrFetchTunnel requests a permanent tunnel from the Worker or loads from cache.
func RegisterOrFetchTunnel(ctx context.Context, workerURL, inviteCode string) (*CFTunnelResult, error) {
	cachePath := filepath.Join(config.GetDataDir(), "cf_tunnel.json")

	// 1. Try reading from cache
	if data, err := os.ReadFile(cachePath); err == nil {
		var cached CFTunnelResult
		if err := json.Unmarshal(data, &cached); err == nil && cached.Token != "" && cached.URL != "" {
			cached.Reused = true
			return &cached, nil
		}
	}

	cleanWorker := strings.TrimRight(strings.TrimSpace(workerURL), "/")
	if cleanWorker == "" {
		return nil, fmt.Errorf("CF_WORKER_URL is empty")
	}

	machineID := GetStableMachineID()
	reqBody := map[string]string{
		"machine_id":  machineID,
		"platform":    fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH),
		"invite_code": inviteCode,
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

	if result.Token == "" || result.URL == "" {
		return nil, fmt.Errorf("invalid worker response, missing token/url: %s", string(respData))
	}

	// 2. Cache result locally
	_ = os.WriteFile(cachePath, respData, 0600)
	return &result, nil
}
