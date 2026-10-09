package proxy

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// 契约测试：项目引用到的 language_server RPC 必须都在基线清单里。
//
// 基线 testdata/ls_rpc_methods.txt 由 scripts/ls-rpc-snapshot.sh --update 从 Antigravity 的
// language_server 二进制抽取。纯离线，CI 上可跑；Antigravity 升级后的真实比对由脚本负责。
// 这里挡住的是另一类错误：代码里写了不存在或拼错的 RPC 名。

// 以下名字不属于 LanguageServerService（来自其它服务或不是 RPC），不参与基线校验。
var nonLSServiceNames = map[string]bool{
	"FindFiles":      true,
	"GetFileDetails": true,
	"IM":             true, // Windows tasklist 的 /IM 参数
	// 旧版客户端方法名，网关会改写成 UpdateConversationAnnotations（proxy_cascade_handlers.go）。
	"SetCascadeTrajectoryMetadata": true,
}

var (
	reLSService = regexp.MustCompile(`LanguageServerService/([A-Za-z]+)`)
	reJSRPC     = regexp.MustCompile(`rpc\("([A-Za-z]+)"`)
	reGoPath    = regexp.MustCompile(`"/([A-Z][A-Za-z]+)"`)
)

func loadRPCBaseline(t *testing.T) map[string]bool {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "ls_rpc_methods.txt"))
	if err != nil {
		t.Fatalf("读取基线失败（先运行 scripts/ls-rpc-snapshot.sh --update）: %v", err)
	}
	defer f.Close()
	set := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		set[line] = true
	}
	if len(set) < 100 {
		t.Fatalf("基线只有 %d 个方法，疑似损坏", len(set))
	}
	return set
}

// collectReferencedRPCs scans Go / Web / Android / iOS sources for the language_server RPC names they use.
func collectReferencedRPCs(t *testing.T) map[string][]string {
	t.Helper()
	root := filepath.Join("..", "..")
	scanDirs := []struct {
		dir string
		ext map[string]bool
	}{
		{"internal", map[string]bool{".go": true}},
		{"cmd", map[string]bool{".go": true}},
		{"web", map[string]bool{".js": true}},
		{filepath.Join("android", "app", "src"), map[string]bool{".kt": true}},
		{"ios", map[string]bool{".swift": true}},
	}

	used := map[string][]string{}
	for _, sd := range scanDirs {
		dir := filepath.Join(root, sd.dir)
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if n := d.Name(); n == "build" || n == "node_modules" || n == "testdata" {
					return fs.SkipDir
				}
				return nil
			}
			if !sd.ext[filepath.Ext(p)] || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			src := string(b)
			res := []*regexp.Regexp{reLSService, reJSRPC}
			if filepath.Ext(p) == ".go" {
				res = append(res, reGoPath)
			}
			for _, re := range res {
				for _, m := range re.FindAllStringSubmatch(src, -1) {
					used[m[1]] = append(used[m[1]], filepath.ToSlash(strings.TrimPrefix(p, root+string(filepath.Separator))))
				}
			}
			return nil
		})
	}
	if len(used) < 10 {
		t.Fatalf("只扫描到 %d 个 RPC 引用，扫描逻辑可能失效", len(used))
	}
	return used
}

func TestReferencedRPCsExistInLanguageServerBaseline(t *testing.T) {
	base := loadRPCBaseline(t)
	used := collectReferencedRPCs(t)

	var missing []string
	for name, files := range used {
		if nonLSServiceNames[name] || base[name] {
			continue
		}
		missing = append(missing, name+"  <- "+files[0])
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("以下 RPC 不在 language_server 基线中（拼错，或 Antigravity 已移除/改名；升级后请先运行 scripts/ls-rpc-snapshot.sh）:\n  %s",
			strings.Join(missing, "\n  "))
	}
}

// 升级自检（capabilities.go）靠「功能 → RPC」表判断哪些功能可用。表必须覆盖源码里实际用到的每个 RPC，
// 否则新增了依赖却忘了登记，升级后该功能消失时手机端不会隐藏入口。
func TestFeatureTableCoversEveryReferencedRPC(t *testing.T) {
	inTable := map[string]bool{}
	for _, f := range Features {
		for _, r := range f.RPCs {
			inTable[r] = true
		}
	}
	var uncovered []string
	for name, files := range collectReferencedRPCs(t) {
		if nonLSServiceNames[name] || inTable[name] {
			continue
		}
		uncovered = append(uncovered, name+"  <- "+files[0])
	}
	sort.Strings(uncovered)
	if len(uncovered) > 0 {
		t.Errorf("以下 RPC 被源码使用，但没有登记到 capabilities.go 的 Features 表（请归入某个功能；核心功能标 Core）:\n  %s",
			strings.Join(uncovered, "\n  "))
	}
}

// 表里登记的 RPC 都必须真实存在于基线，避免拼错后永远显示「功能不可用」。
func TestFeatureTableRPCsExistInBaseline(t *testing.T) {
	base := loadRPCBaseline(t)
	seenID := map[string]bool{}
	for _, f := range Features {
		if f.ID == "" || seenID[f.ID] {
			t.Errorf("功能 ID 为空或重复: %q", f.ID)
		}
		seenID[f.ID] = true
		if len(f.RPCs) == 0 {
			t.Errorf("功能 %s 没有登记任何 RPC", f.ID)
		}
		for _, r := range f.RPCs {
			if !base[r] {
				t.Errorf("功能 %s 依赖的 %s 不在 language_server 基线中", f.ID, r)
			}
		}
	}
	// 客户端按这些 ID 隐藏入口；改名会让手机端静默失效
	for _, id := range []string{"core", "interaction", "search", "export", "changes", "revert", "slash"} {
		if !seenID[id] {
			t.Errorf("客户端依赖的功能 ID %q 不存在", id)
		}
	}
}
