package proxy

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/inspector"
)

// 升级自检。
//
// 整个项目建立在逆向之上，Antigravity 升级后 language_server 的 RPC 可能新增、改名或消失。
// 网关在每次连上（新的）language_server 实例时，读取其二进制里的 LanguageServerServiceHandler.<方法> 清单
// （只读文件，不发任何请求，没有副作用），对照下面的「功能 → 依赖的 RPC」表，得出哪些功能不可用，
// 通过 /gateway/status 的 compat 字段告诉客户端：
//   - 不可用的功能：客户端隐藏对应入口（搜索、导出、Changes、撤回、斜杠命令等）；
//   - 核心功能缺失：客户端在首页显示「网关与当前 Antigravity 版本不兼容」提示条。
//
// 读不到二进制（Linux 无头/daemon 模式、权限不足等）时一律视为「未知」：不隐藏任何入口。

// Feature groups the language_server RPCs one user-visible capability depends on.
type Feature struct {
	// ID is the stable identifier clients use to hide entries ("search", "slash", ...).
	ID string
	// Core features are the basics (list / read / send / stream / approve); losing one makes the app unusable.
	Core bool
	RPCs []string
}

// Features is the single source of truth for what the gateway and the apps need from language_server.
// ls_rpc_contract_test.go verifies that every RPC referenced anywhere in the source tree appears here.
var Features = []Feature{
	{ID: "core", Core: true, RPCs: []string{"GetAllCascadeTrajectories", "GetCascadeTrajectory", "SendUserCascadeMessage", "StartCascade", "StreamAgentStateUpdates", "GetStatus"}},
	{ID: "interaction", Core: true, RPCs: []string{"HandleCascadeUserInteraction", "CancelCascadeInvocation"}},
	{ID: "search", RPCs: []string{"SearchConversations"}},
	{ID: "export", RPCs: []string{"ConvertTrajectoryToMarkdown"}},
	{ID: "changes", RPCs: []string{"GetRevertPreview"}},
	{ID: "revert", RPCs: []string{"GetRevertPreview", "RevertToCascadeStep"}},
	{ID: "slash", RPCs: []string{"GetSlashCommands"}},
	{ID: "models", RPCs: []string{"GetAvailableModels", "JetboxWriteState"}}, // 网关有内置列表兜底，客户端不据此隐藏
	{ID: "queue", RPCs: []string{"DeleteAgentMessage"}},
	{ID: "tasks", RPCs: []string{"CancelCascadeSteps", "ForceStopCascadeTree"}},
	{ID: "manage", RPCs: []string{"DeleteCascadeTrajectory", "UpdateConversationAnnotations", "LoadTrajectory"}},
	{ID: "projects", RPCs: []string{"ReadDir", "ReadProjects", "StatUri"}},
	{ID: "account", RPCs: []string{"GetUserStatus", "GetAuthStatus", "GetCascadeNuxes", "RetrieveUserQuotaSummary"}},
}

// Compatibility is the result of comparing the running language_server with what the gateway needs.
type Compatibility struct {
	// Version is the Antigravity version of the running instance ("" when unknown).
	Version string `json:"version,omitempty"`
	// Checked is false when the binary could not be inspected; clients then assume everything works.
	Checked bool `json:"checked"`
	// CoreOK is false when a core feature is missing — the apps should say the gateway is incompatible.
	CoreOK bool `json:"coreOk"`
	// Unavailable lists the IDs of features that miss at least one RPC; clients hide their entry points.
	Unavailable []string `json:"unavailable,omitempty"`
	// MissingRPCs lists the absent method names (for logs and bug reports).
	MissingRPCs []string `json:"missingRpcs,omitempty"`
}

// unknownCompatibility is what clients see before (or without) a successful inspection.
func unknownCompatibility(version string) Compatibility {
	return Compatibility{Version: version, Checked: false, CoreOK: true}
}

// evaluateCompatibility maps the set of methods present in language_server to feature availability.
func evaluateCompatibility(methods map[string]bool, version string) Compatibility {
	c := Compatibility{Version: version, Checked: true, CoreOK: true}
	missing := map[string]bool{}
	for _, f := range Features {
		var featureMissing bool
		for _, rpc := range f.RPCs {
			if !methods[rpc] {
				featureMissing = true
				missing[rpc] = true
			}
		}
		if featureMissing {
			c.Unavailable = append(c.Unavailable, f.ID)
			if f.Core {
				c.CoreOK = false
			}
		}
	}
	for rpc := range missing {
		c.MissingRPCs = append(c.MissingRPCs, rpc)
	}
	sort.Strings(c.MissingRPCs)
	return c
}

// ---------------------------------------------------------------------------
// reading the binary
// ---------------------------------------------------------------------------

var handlerMarker = []byte("LanguageServerServiceHandler.")

const (
	scanChunkSize = 8 << 20 // 8 MiB
	scanOverlap   = 256     // longer than marker + any method name, so a name cut by a chunk edge is still seen whole
)

func isMethodNameByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_'
}

// scanMethodNames reads r in chunks and collects every `LanguageServerServiceHandler.<Method>` it finds.
// The Go binary embeds these names as plain strings (connect-go handler symbols).
func scanMethodNames(r io.Reader, chunkSize int) (map[string]bool, error) {
	methods := map[string]bool{}
	buf := make([]byte, 0, chunkSize+scanOverlap)
	tmp := make([]byte, chunkSize)
	eof := false
	for !eof {
		n, err := io.ReadFull(r, tmp)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			eof = true
		} else if err != nil {
			return nil, err
		}
		buf = append(buf, tmp[:n]...)

		// 只接受「起点落在本块安全区内」的匹配，尾部 scanOverlap 字节留到下一块（拼上后再完整匹配）
		safeEnd := len(buf)
		if !eof {
			safeEnd = len(buf) - scanOverlap
			if safeEnd < 0 {
				safeEnd = 0
			}
		}
		for off := 0; off < safeEnd; {
			i := bytes.Index(buf[off:], handlerMarker)
			if i < 0 {
				break
			}
			start := off + i + len(handlerMarker)
			if off+i >= safeEnd {
				break
			}
			end := start
			for end < len(buf) && end-start < 80 && isMethodNameByte(buf[end]) {
				end++
			}
			if end > start {
				methods[string(buf[start:end])] = true
			}
			off = end
		}

		if !eof && len(buf) <= scanOverlap {
			continue // 还不够一个安全区，继续累积
		}
		if !eof {
			// 保留尾部重叠区，丢弃已处理部分
			keep := make([]byte, scanOverlap)
			copy(keep, buf[len(buf)-scanOverlap:])
			buf = append(buf[:0], keep...)
		}
	}
	return methods, nil
}

// scanBinaryMethods reads the language_server binary at path.
func scanBinaryMethods(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	methods, err := scanMethodNames(f, scanChunkSize)
	if err != nil {
		return nil, err
	}
	if len(methods) == 0 {
		return nil, fmt.Errorf("no LanguageServerService methods found in %s", path)
	}
	return methods, nil
}

// ---------------------------------------------------------------------------
// proxy integration
// ---------------------------------------------------------------------------

// lookupProcessDetails is a seam for tests.
var lookupProcessDetails = inspector.LookupProcessDetails

// scanMethods is a seam for tests.
var scanMethods = scanBinaryMethods

type compatState struct {
	mu      sync.RWMutex
	current Compatibility
	key     string // identity of the instance the result belongs to
	ready   bool
}

func (c *compatState) get() (Compatibility, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.current, c.ready
}

func (c *compatState) set(key string, v Compatibility) {
	c.mu.Lock()
	c.current, c.key, c.ready = v, key, true
	c.mu.Unlock()
}

func (c *compatState) hasKey(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready && c.key == key
}

// refreshCompatibility inspects the instance (once per PID/version/binary) and stores the verdict.
// It is cheap to call repeatedly and never blocks request handling.
func (p *Proxy) refreshCompatibility(info inspector.InstanceInfo) {
	details := lookupProcessDetails(info.PID)
	version := info.Version
	if version == "" {
		version = details.Version
	}

	var stamp string
	if details.ExecutablePath != "" {
		if st, err := os.Stat(details.ExecutablePath); err == nil {
			stamp = fmt.Sprintf("%d:%d", st.Size(), st.ModTime().UnixNano())
		}
	}
	key := fmt.Sprintf("%d|%s|%s|%s", info.PID, version, details.ExecutablePath, stamp)
	if p.compat.hasKey(key) {
		return
	}

	if details.ExecutablePath == "" || stamp == "" {
		p.compat.set(key, unknownCompatibility(version))
		slog.Info("[Compat] 无法定位 language_server 二进制，跳过升级自检（所有功能按可用处理）", "version", version)
		return
	}

	start := time.Now()
	methods, err := scanMethods(details.ExecutablePath)
	if err != nil {
		p.compat.set(key, unknownCompatibility(version))
		slog.Warn("[Compat] 读取 language_server 方法清单失败，跳过升级自检", "err", err)
		return
	}
	result := evaluateCompatibility(methods, version)
	p.compat.set(key, result)

	label := version
	if label == "" {
		label = "未知版本"
	}
	switch {
	case !result.CoreOK:
		slog.Error(fmt.Sprintf("[Compat] ❌ Antigravity %s 缺少核心接口，网关与它不兼容: %s（需要升级 mgy）", label, strings.Join(result.MissingRPCs, ", ")))
	case len(result.Unavailable) > 0:
		slog.Warn(fmt.Sprintf("[Compat] ⚠️ Antigravity %s 缺少部分接口，相关功能已在客户端隐藏: %s（缺少 %s）",
			label, strings.Join(result.Unavailable, ", "), strings.Join(result.MissingRPCs, ", ")))
	default:
		slog.Info(fmt.Sprintf("[Compat] ✅ Antigravity %s：依赖的 %d 个接口均存在（扫描耗时 %s）", label, countFeatureRPCs(), time.Since(start).Round(time.Millisecond)))
	}
}

func countFeatureRPCs() int {
	seen := map[string]bool{}
	for _, f := range Features {
		for _, r := range f.RPCs {
			seen[r] = true
		}
	}
	return len(seen)
}

// currentCompatibility is what /gateway/status reports.
func (p *Proxy) currentCompatibility(info *inspector.InstanceInfo) *Compatibility {
	if c, ok := p.compat.get(); ok {
		return &c
	}
	version := ""
	if info != nil {
		version = info.Version
	}
	c := unknownCompatibility(version)
	return &c
}
