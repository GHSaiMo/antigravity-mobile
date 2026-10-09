package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"antigravity-mobile/internal/inspector"
)

// ---- scanning the binary ------------------------------------------------------

func fakeBinary(methods ...string) []byte {
	var b bytes.Buffer
	b.WriteString("\x7fELF-ish header \x00\x00 random go symbols main.main runtime.gopanic ")
	for _, m := range methods {
		b.WriteString("\x00junk\x00connect.LanguageServerServiceHandler.")
		b.WriteString(m)
		b.WriteString("\x00more junk \x01\x02 exa.language_server_pb.LanguageServerService/Other\x00")
	}
	return b.Bytes()
}

func TestScanMethodNames(t *testing.T) {
	got, err := scanMethodNames(bytes.NewReader(fakeBinary("SendUserCascadeMessage", "GetStatus", "Search_Conversations2")), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"SendUserCascadeMessage", "GetStatus", "Search_Conversations2"} {
		if !got[want] {
			t.Errorf("missing %s in %v", want, got)
		}
	}
	// 「服务/方法」路径形式不算（Go 字符串拼接会粘连噪声，只认 Handler 符号）
	if got["Other"] {
		t.Error("only LanguageServerServiceHandler.<Method> symbols count")
	}
}

func TestScanMethodNamesAcrossChunkBoundaries(t *testing.T) {
	// 在不同偏移量放一个方法名，并用很小的分块反复扫描：无论名字被分块边界切在哪里都必须完整找到，且不重复/不残缺
	names := []string{"SendUserCascadeMessage", "GetAllCascadeTrajectories", "StreamAgentStateUpdates"}
	base := fakeBinary(names...)
	for pad := 0; pad < 700; pad += 37 {
		data := append(bytes.Repeat([]byte{'x'}, pad), base...)
		for _, chunk := range []int{1, 7, 100, 300, 513, 4096} {
			got, err := scanMethodNames(bytes.NewReader(data), chunk)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(names) {
				t.Fatalf("pad=%d chunk=%d: got %v", pad, chunk, got)
			}
			for _, n := range names {
				if !got[n] {
					t.Fatalf("pad=%d chunk=%d: lost %s (got %v)", pad, chunk, n, got)
				}
			}
		}
	}
}

func TestScanMethodNamesEmptyAndGarbage(t *testing.T) {
	for _, in := range [][]byte{nil, []byte("hello world"), bytes.Repeat([]byte("LanguageServerServiceHandler"), 10)} {
		got, err := scanMethodNames(bytes.NewReader(in), 64)
		if err != nil || len(got) != 0 {
			t.Errorf("input %q => %v, %v", in, got, err)
		}
	}
	// 标记在文件末尾、后面没有名字
	got, _ := scanMethodNames(bytes.NewReader([]byte("xx LanguageServerServiceHandler.")), 8)
	if len(got) != 0 {
		t.Errorf("dangling marker must not yield a method: %v", got)
	}
}

func TestScanBinaryMethodsRejectsBinaryWithoutMethods(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "not_a_language_server")
	if err := os.WriteFile(p, []byte("nothing useful"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := scanBinaryMethods(p); err == nil {
		t.Error("a file without handler symbols must be an error (→ unknown), not an empty method set")
	}
	if _, err := scanBinaryMethods(filepath.Join(dir, "missing")); err == nil {
		t.Error("missing file must be an error")
	}
}

// ---- evaluating ------------------------------------------------------------

func allMethods() map[string]bool {
	m := map[string]bool{}
	for _, f := range Features {
		for _, r := range f.RPCs {
			m[r] = true
		}
	}
	return m
}

func TestEvaluateCompatibilityAllPresent(t *testing.T) {
	c := evaluateCompatibility(allMethods(), "2.22.0")
	if !c.Checked || !c.CoreOK || len(c.Unavailable) != 0 || len(c.MissingRPCs) != 0 || c.Version != "2.22.0" {
		t.Errorf("%+v", c)
	}
}

func TestEvaluateCompatibilityOptionalFeatureMissing(t *testing.T) {
	m := allMethods()
	delete(m, "SearchConversations")
	c := evaluateCompatibility(m, "2.30.0")
	if !c.CoreOK {
		t.Error("losing search must not make the gateway incompatible")
	}
	if strings.Join(c.Unavailable, ",") != "search" || strings.Join(c.MissingRPCs, ",") != "SearchConversations" {
		t.Errorf("%+v", c)
	}
}

func TestEvaluateCompatibilitySharedRPCHidesEveryDependentFeature(t *testing.T) {
	m := allMethods()
	delete(m, "GetRevertPreview") // changes 与 revert 都依赖它
	c := evaluateCompatibility(m, "")
	got := append([]string(nil), c.Unavailable...)
	sort.Strings(got)
	if strings.Join(got, ",") != "changes,revert" || !c.CoreOK {
		t.Errorf("%+v", c)
	}
}

func TestEvaluateCompatibilityCoreMissing(t *testing.T) {
	for _, rpc := range []string{"SendUserCascadeMessage", "GetAllCascadeTrajectories", "StreamAgentStateUpdates", "HandleCascadeUserInteraction"} {
		m := allMethods()
		delete(m, rpc)
		c := evaluateCompatibility(m, "9.9.9")
		if c.CoreOK {
			t.Errorf("missing %s must clear CoreOK", rpc)
		}
		if len(c.Unavailable) == 0 || len(c.MissingRPCs) != 1 || c.MissingRPCs[0] != rpc {
			t.Errorf("%s => %+v", rpc, c)
		}
	}
}

func TestUnknownCompatibilityHidesNothing(t *testing.T) {
	c := unknownCompatibility("2.22.0")
	if c.Checked || !c.CoreOK || len(c.Unavailable) != 0 {
		t.Errorf("unknown must be permissive: %+v", c)
	}
}

// ---- proxy integration ---------------------------------------------------------

func withSeams(t *testing.T, details inspector.ProcessDetails, methods map[string]bool, scanErr error, scans *int) {
	t.Helper()
	origLookup, origScan := lookupProcessDetails, scanMethods
	lookupProcessDetails = func(pid int) inspector.ProcessDetails { return details }
	scanMethods = func(path string) (map[string]bool, error) {
		if scans != nil {
			*scans++
		}
		return methods, scanErr
	}
	t.Cleanup(func() { lookupProcessDetails, scanMethods = origLookup, origScan })
}

func tempBinary(t *testing.T) string {
	p := filepath.Join(t.TempDir(), "language_server")
	if err := os.WriteFile(p, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRefreshCompatibilityReportsMissingFeatures(t *testing.T) {
	bin := tempBinary(t)
	m := allMethods()
	delete(m, "ConvertTrajectoryToMarkdown")
	delete(m, "GetSlashCommands")
	withSeams(t, inspector.ProcessDetails{ExecutablePath: bin, Version: "2.40.1"}, m, nil, nil)

	p := &Proxy{}
	p.refreshCompatibility(inspector.InstanceInfo{PID: 4242})
	c, ready := p.compat.get()
	if !ready || !c.Checked || !c.CoreOK || c.Version != "2.40.1" {
		t.Fatalf("%+v ready=%v", c, ready)
	}
	got := append([]string(nil), c.Unavailable...)
	sort.Strings(got)
	if strings.Join(got, ",") != "export,slash" {
		t.Errorf("unavailable = %v", got)
	}
}

func TestRefreshCompatibilityScansOncePerInstance(t *testing.T) {
	bin := tempBinary(t)
	scans := 0
	withSeams(t, inspector.ProcessDetails{ExecutablePath: bin, Version: "2.22.0"}, allMethods(), nil, &scans)
	p := &Proxy{}
	info := inspector.InstanceInfo{PID: 1}
	p.refreshCompatibility(info)
	p.refreshCompatibility(info)
	p.refreshCompatibility(inspector.InstanceInfo{PID: 1, Port: 9999}) // 只是端口变了，同一个进程
	if scans != 1 {
		t.Errorf("same process/binary must be scanned once, got %d", scans)
	}
	p.refreshCompatibility(inspector.InstanceInfo{PID: 2}) // 新进程（升级后重启）
	if scans != 2 {
		t.Errorf("a new process must be re-checked, got %d", scans)
	}
	// 二进制被替换（升级覆盖安装，PID 不变也不可能，但路径下的文件变了）
	if err := os.WriteFile(bin, []byte("a different, longer binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	p.refreshCompatibility(inspector.InstanceInfo{PID: 2})
	if scans != 3 {
		t.Errorf("a replaced binary must be re-checked, got %d", scans)
	}
}

func TestRefreshCompatibilityUnknownWhenBinaryNotFound(t *testing.T) {
	withSeams(t, inspector.ProcessDetails{Version: "2.22.0"}, nil, nil, nil)
	p := &Proxy{}
	p.refreshCompatibility(inspector.InstanceInfo{PID: 7})
	c, _ := p.compat.get()
	if c.Checked || !c.CoreOK || len(c.Unavailable) != 0 || c.Version != "2.22.0" {
		t.Errorf("no binary path must mean 'unknown', not 'broken': %+v", c)
	}
}

func TestRefreshCompatibilityUnknownWhenScanFails(t *testing.T) {
	bin := tempBinary(t)
	withSeams(t, inspector.ProcessDetails{ExecutablePath: bin}, nil, os.ErrPermission, nil)
	p := &Proxy{}
	p.refreshCompatibility(inspector.InstanceInfo{PID: 7})
	c, _ := p.compat.get()
	if c.Checked || !c.CoreOK || len(c.Unavailable) != 0 {
		t.Errorf("unreadable binary must mean 'unknown': %+v", c)
	}
}

func TestRefreshCompatibilityPrefersDaemonVersion(t *testing.T) {
	bin := tempBinary(t)
	withSeams(t, inspector.ProcessDetails{ExecutablePath: bin, Version: "from-ps"}, allMethods(), nil, nil)
	p := &Proxy{}
	p.refreshCompatibility(inspector.InstanceInfo{PID: 7, Version: "from-daemon"})
	if c, _ := p.compat.get(); c.Version != "from-daemon" {
		t.Errorf("version = %q", c.Version)
	}
}

func TestStatusEndpointCarriesCompatButNoPaths(t *testing.T) {
	bin := tempBinary(t)
	m := allMethods()
	delete(m, "SearchConversations")
	withSeams(t, inspector.ProcessDetails{ExecutablePath: bin, Version: "2.30.0"}, m, nil, nil)

	insp := inspector.NewInspector(5 * time.Second)
	p := NewProxy(insp)
	p.refreshCompatibility(inspector.InstanceInfo{PID: 99})

	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/gateway/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var body struct {
		Compat *Compatibility `json:"compat"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Compat == nil {
		t.Fatalf("compat missing: %s", rec.Body.String())
	}
	if !body.Compat.Checked || !body.Compat.CoreOK || strings.Join(body.Compat.Unavailable, ",") != "search" || body.Compat.Version != "2.30.0" {
		t.Errorf("%+v", body.Compat)
	}
	if strings.Contains(rec.Body.String(), bin) || strings.Contains(rec.Body.String(), "language_server\"") {
		t.Errorf("status must not leak the binary path: %s", rec.Body.String())
	}
}

func TestStatusEndpointBeforeAnyInspectionIsPermissive(t *testing.T) {
	p := NewProxy(inspector.NewInspector(5 * time.Second))
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/gateway/status", nil))
	var body struct {
		Compat *Compatibility `json:"compat"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Compat == nil || body.Compat.Checked || !body.Compat.CoreOK || len(body.Compat.Unavailable) != 0 {
		t.Errorf("before inspection clients must assume everything works: %s", rec.Body.String())
	}
}

// 真实二进制：本机装了 Antigravity 才跑，用来确认扫描器对真实的 150MB 文件有效，且当前 2.22.0 满足全部依赖。
func TestScanRealLanguageServerWhenInstalled(t *testing.T) {
	var candidates = []string{
		"/Applications/Antigravity.app/Contents/Resources/bin/language_server",
	}
	var path string
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			path = c
			break
		}
	}
	if path == "" {
		t.Skip("Antigravity not installed")
	}
	methods, err := scanBinaryMethods(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(methods) < 200 {
		t.Fatalf("only %d methods found in the real binary", len(methods))
	}
	c := evaluateCompatibility(methods, "")
	if !c.CoreOK {
		t.Errorf("the installed Antigravity must satisfy the core features: %+v", c)
	}
	t.Logf("real binary: %d methods, unavailable=%v missing=%v", len(methods), c.Unavailable, c.MissingRPCs)
}
