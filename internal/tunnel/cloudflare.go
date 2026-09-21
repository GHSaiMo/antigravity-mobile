package tunnel

import (
	"archive/tar"
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
	cmd     *exec.Cmd
	mu      sync.Mutex
	running bool
}

// NewCloudflareTunnel creates a supervisor for a Cloudflare Tunnel.
func NewCloudflareTunnel(res *CFTunnelResult) *CloudflareTunnel {
	return &CloudflareTunnel{
		result: res,
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

	cmd := exec.CommandContext(ctx, binPath, "tunnel", "run", "--token", t.result.Token)
	// Do not attach stdin. Divert stderr to logger with prefix
	stderr, err := cmd.StderrPipe()
	if err == nil {
		go func() {
			buf := make([]byte, 1024)
			for {
				n, rErr := stderr.Read(buf)
				if n > 0 {
					line := strings.TrimSpace(string(buf[:n]))
					// Filter verbose cloudflared heartbeats, print errors / connections
					if strings.Contains(line, "Registered tunnel connection") ||
						strings.Contains(line, "Connection") && strings.Contains(line, "registered") {
						log.Printf("☁️  [Cloudflare] %s", line)
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
	log.Printf("☁️  [Cloudflare] Daemon started (PID %d) -> %s", cmd.Process.Pid, t.result.URL)

	go func() {
		_ = cmd.Wait()
		t.mu.Lock()
		t.running = false
		t.mu.Unlock()
	}()

	return nil
}

// Stop terminates the running cloudflared process.
func (t *CloudflareTunnel) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.running || t.cmd == nil || t.cmd.Process == nil {
		return
	}
	_ = t.cmd.Process.Kill()
	t.running = false
}

// EnsureCloudflaredBinary checks for cloudflared in PATH or downloads it automatically.
func EnsureCloudflaredBinary(ctx context.Context) (string, error) {
	// 1. Check system PATH
	if bin, err := exec.LookPath("cloudflared"); err == nil {
		return bin, nil
	}

	// 2. Check ~/.multigravity/bin/cloudflared
	binDir := filepath.Join(config.GetDataDir(), "bin")
	binName := "cloudflared"
	if runtime.GOOS == "windows" {
		binName = "cloudflared.exe"
	}
	targetPath := filepath.Join(binDir, binName)

	if fi, err := os.Stat(targetPath); err == nil && !fi.IsDir() && fi.Size() > 1024*1024 {
		return targetPath, nil
	}

	// 3. Needs download
	_ = os.MkdirAll(binDir, 0755)
	log.Printf("⏬ 正在初始化 Cloudflare 穿透引擎 (初次拉取约需 2~3 秒)...")

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
	default:
		return nil
	}

	var urls []string
	for _, fn := range baseNames {
		ghURL := fmt.Sprintf("https://github.com/cloudflare/cloudflared/releases/latest/download/%s", fn)
		// 优先使用国内加速源
		urls = append(urls, "https://ghfast.top/"+ghURL)
		// 官方直链兜底
		urls = append(urls, ghURL)
	}
	return urls
}

func downloadAndInstallBinary(ctx context.Context, downloadURL, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Multigravity-AutoInstaller/1.0")

	client := &http.Client{Timeout: 30 * time.Second}
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
