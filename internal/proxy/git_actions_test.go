package proxy

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("git 不在 PATH 中，无法验证 Git 提交逻辑: %v", err)
	}
}

// newTestRepo creates an isolated repo with one commit; HOME is redirected so the developer's
// global git config / hooks never leak into the test.
func newTestRepo(t *testing.T) string {
	t.Helper()
	requireGit(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))

	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		out, err := runGit(context.Background(), dir, gitStatusTimeout, args...)
		if err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	run("config", "commit.gpgsign", "false")
	writeTestFile(t, dir, "a.txt", "a\n")
	writeTestFile(t, dir, "b.txt", "b\n")
	run("add", ".")
	run("commit", "-q", "-m", "init")
	root, err := gitRepoRoot(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseGitStatus(t *testing.T) {
	raw := "## main...origin/main [ahead 2, behind 1]\x00" +
		" M a.txt\x00" +
		"A  new dir/b.go\x00" +
		"R  renamed.txt\x00old.txt\x00" +
		"?? untracked.txt\x00" +
		" D gone.txt\x00" +
		"UU conflict.txt\x00"
	st := parseGitStatus(raw)

	if st.Branch != "main" || st.Upstream != "origin/main" || st.Ahead != 2 || st.Behind != 1 {
		t.Fatalf("branch line parsed wrong: %+v", st)
	}
	want := []struct {
		path, status string
		staged       bool
	}{
		{"a.txt", "MODIFIED", false},
		{"new dir/b.go", "ADDED", true},
		{"renamed.txt", "RENAMED", true},
		{"untracked.txt", "UNTRACKED", false},
		{"gone.txt", "DELETED", false},
		{"conflict.txt", "CONFLICT", true},
	}
	if len(st.Files) != len(want) {
		t.Fatalf("got %d files, want %d: %+v", len(st.Files), len(want), st.Files)
	}
	for i, w := range want {
		f := st.Files[i]
		if f.Path != w.path || f.Status != w.status || f.Staged != w.staged {
			t.Errorf("file %d = %+v, want %+v", i, f, w)
		}
	}
	if st.Files[2].OrigPath != "old.txt" {
		t.Errorf("rename source = %q", st.Files[2].OrigPath)
	}
	if st.Clean {
		t.Error("Clean should be false")
	}
}

func TestParseGitBranchLine(t *testing.T) {
	cases := map[string]GitStatusResponse{
		"main":                              {Branch: "main"},
		"No commits yet on trunk":           {Branch: "trunk"},
		"HEAD (no branch)":                  {Branch: "HEAD", Detached: true},
		"dev...origin/dev":                  {Branch: "dev", Upstream: "origin/dev"},
		"dev...origin/dev [gone]":           {Branch: "dev", Upstream: "origin/dev"},
		"feat/x...origin/feat/x [behind 3]": {Branch: "feat/x", Upstream: "origin/feat/x", Behind: 3},
	}
	for line, want := range cases {
		var got GitStatusResponse
		parseGitBranchLine(line, &got)
		if got.Branch != want.Branch || got.Upstream != want.Upstream || got.Ahead != want.Ahead ||
			got.Behind != want.Behind || got.Detached != want.Detached {
			t.Errorf("%q => %+v, want %+v", line, got, want)
		}
	}
}

func TestGitCommitSelectedPathsOnly(t *testing.T) {
	root := newTestRepo(t)
	ctx := context.Background()
	writeTestFile(t, root, "a.txt", "a2\n")
	writeTestFile(t, root, "b.txt", "b2\n")
	writeTestFile(t, root, "sub/new.txt", "n\n")

	res, err := gitCommit(ctx, root, "only a and new", []string{"a.txt", "sub/new.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Committed || res.CommitID == "" {
		t.Fatalf("unexpected result %+v", res)
	}
	st, err := gitStatus(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Files) != 1 || st.Files[0].Path != "b.txt" {
		t.Fatalf("b.txt should remain uncommitted, got %+v", st.Files)
	}
	msg, _ := runGit(ctx, root, gitStatusTimeout, "log", "-1", "--format=%s")
	if msg != "only a and new" {
		t.Errorf("commit message = %q", msg)
	}
}

func TestGitCommitAllAndDeletions(t *testing.T) {
	root := newTestRepo(t)
	ctx := context.Background()
	if err := os.Remove(filepath.Join(root, "a.txt")); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, "c.txt", "c\n")

	if _, err := gitCommit(ctx, root, "all", nil); err != nil {
		t.Fatal(err)
	}
	st, _ := gitStatus(ctx, root)
	if !st.Clean {
		t.Fatalf("working tree should be clean: %+v", st.Files)
	}
}

func TestGitCommitRenameCarriesSource(t *testing.T) {
	root := newTestRepo(t)
	ctx := context.Background()
	if _, err := runGit(ctx, root, gitStatusTimeout, "mv", "a.txt", "renamed.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := gitCommit(ctx, root, "rename", []string{"renamed.txt"}); err != nil {
		t.Fatal(err)
	}
	st, _ := gitStatus(ctx, root)
	if !st.Clean {
		t.Fatalf("rename should commit both sides, left: %+v", st.Files)
	}
}

func TestGitCommitRejections(t *testing.T) {
	root := newTestRepo(t)
	ctx := context.Background()

	if _, err := gitCommit(ctx, root, "nothing", nil); err == nil || !strings.Contains(err.Error(), "没有可提交") {
		t.Errorf("clean tree should be rejected, got %v", err)
	}

	writeTestFile(t, root, "a.txt", "changed\n")
	bad := [][]string{
		{"-A"},              // option injection
		{"--all"},           // option injection
		{"/etc/passwd"},     // absolute path
		{"../outside"},      // not in status
		{"unchanged-b.txt"}, // not changed
		{"b.txt"},           // tracked but unchanged
	}
	for _, paths := range bad {
		if _, err := gitCommit(ctx, root, "x", paths); err == nil {
			t.Errorf("paths %v should be rejected", paths)
		}
	}
	if _, err := gitCommit(ctx, root, "   ", nil); err == nil {
		t.Error("blank message should be rejected")
	}
	if _, err := gitCommit(ctx, root, strings.Repeat("x", gitMaxMessageLen+1), nil); err == nil {
		t.Error("oversized message should be rejected")
	}
}

func TestGitCommitConflictBlocked(t *testing.T) {
	root := newTestRepo(t)
	ctx := context.Background()
	mustGit := func(args ...string) {
		t.Helper()
		if out, err := runGit(ctx, root, gitCommitTimeout, args...); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	mustGit("checkout", "-q", "-b", "other")
	writeTestFile(t, root, "a.txt", "other\n")
	mustGit("commit", "-qam", "other change")
	mustGit("checkout", "-q", "main")
	writeTestFile(t, root, "a.txt", "main\n")
	mustGit("commit", "-qam", "main change")
	// 合并必然冲突，退出码非 0 是预期的
	_, _ = runGit(ctx, root, gitCommitTimeout, "merge", "other")

	if _, err := gitCommit(ctx, root, "should not commit", nil); err == nil || !strings.Contains(err.Error(), "冲突") {
		t.Fatalf("conflicts must block commit, got %v", err)
	}
}

func TestGitRepoRootNotARepo(t *testing.T) {
	requireGit(t)
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(t.TempDir()))
	if _, err := gitRepoRoot(context.Background(), t.TempDir()); err != errNotGitRepo {
		t.Fatalf("got %v, want errNotGitRepo", err)
	}
}

func TestGitPushWithoutRemoteFailsCleanly(t *testing.T) {
	root := newTestRepo(t)
	_, err := gitPush(context.Background(), root)
	if err == nil {
		t.Fatal("push without remote must fail")
	}
}

func TestGitPushToLocalBareRemote(t *testing.T) {
	root := newTestRepo(t)
	ctx := context.Background()
	bare := t.TempDir()
	if out, err := runGit(ctx, bare, gitStatusTimeout, "init", "-q", "--bare", "-b", "main"); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if out, err := runGit(ctx, root, gitStatusTimeout, "remote", "add", "origin", bare); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	// 首次推送自动设置 upstream
	if _, err := gitPush(ctx, root); err != nil {
		t.Fatal(err)
	}
	st, _ := gitStatus(ctx, root)
	if st.Upstream != "origin/main" || st.Ahead != 0 {
		t.Fatalf("upstream not set: %+v", st)
	}
	writeTestFile(t, root, "a.txt", "again\n")
	if _, err := gitCommit(ctx, root, "second", nil); err != nil {
		t.Fatal(err)
	}
	if st, _ = gitStatus(ctx, root); st.Ahead != 1 {
		t.Fatalf("ahead = %d, want 1", st.Ahead)
	}
	if _, err := gitPush(ctx, root); err != nil {
		t.Fatal(err)
	}
	if st, _ = gitStatus(ctx, root); st.Ahead != 0 {
		t.Fatalf("ahead after push = %d", st.Ahead)
	}
}

func TestWorkspaceURIsFromSummaries(t *testing.T) {
	body := []byte(`{"trajectorySummaries":{
	  "abc-1":{"workspaces":[{"workspaceFolderAbsoluteUri":"file:///Users/me/proj"},{"workspaceFolderAbsoluteUri":"file:///opt/other"}]},
	  "abc-2":{"workspaces":[]}}}`)
	got := workspaceURIsFromSummaries(body, "abc-1")
	if len(got) != 2 || got[0] != "file:///Users/me/proj" {
		t.Fatalf("got %v", got)
	}
	if got := workspaceURIsFromSummaries(body, "abc-2"); len(got) != 0 {
		t.Fatalf("empty workspaces => %v", got)
	}
	if got := workspaceURIsFromSummaries([]byte("not json"), "abc-1"); got != nil {
		t.Fatalf("bad json => %v", got)
	}
}

func TestFileURIToLocalPath(t *testing.T) {
	if _, err := fileURIToLocalPath("https://example.com/x"); err == nil {
		t.Error("non-file scheme must be rejected")
	}
	if _, err := fileURIToLocalPath("file://"); err == nil {
		t.Error("empty path must be rejected")
	}
	got, err := fileURIToLocalPath("file:///Users/me/my%20proj")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(filepath.ToSlash(got), "/Users/me/my proj") {
		t.Errorf("got %q", got)
	}
}
