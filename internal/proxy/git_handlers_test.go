package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 以下两个辅助函数被本文件和后续的 handler 测试共用。
func newFakeLSProxy(t *testing.T, handler http.HandlerFunc) *Proxy {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	p := &Proxy{shortClient: srv.Client(), mediumClient: srv.Client(), longClient: srv.Client()}
	p.SetTestUpstream(srv.Listener.Addr().(*net.TCPAddr).Port, "test-token")
	return p
}

func postJSON(t *testing.T, h http.HandlerFunc, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

// fakeLSWithWorkspace answers GetAllCascadeTrajectories with one cascade bound to dir.

func fakeLSWithWorkspace(t *testing.T, cascadeID, dirURI string) *Proxy {
	return newFakeLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"trajectorySummaries": map[string]interface{}{
				cascadeID: map[string]interface{}{
					"workspaces": []map[string]interface{}{{"workspaceFolderAbsoluteUri": dirURI}},
				},
			},
		})
	})
}

func TestHandleGitStatusAndCommit_EndToEnd(t *testing.T) {
	root := newTestRepo(t)
	writeTestFile(t, root, "a.txt", "changed\n")
	writeTestFile(t, root, "new.txt", "n\n")
	p := fakeLSWithWorkspace(t, "casc-1", "file://"+root)

	w := postJSON(t, p.HandleGitStatus, "/gateway/git/status", `{"cascadeId":"casc-1"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d %s", w.Code, w.Body.String())
	}
	var st GitStatusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.Branch != "main" || len(st.Files) != 2 || st.Clean {
		t.Fatalf("unexpected status: %+v", st)
	}

	// 非法路径被拒
	w = postJSON(t, p.HandleGitCommit, "/gateway/git/commit", `{"cascadeId":"casc-1","message":"m","paths":["--all"]}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad path => %d %s", w.Code, w.Body.String())
	}

	w = postJSON(t, p.HandleGitCommit, "/gateway/git/commit", `{"cascadeId":"casc-1","message":"feat: add","paths":["new.txt"]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("commit: %d %s", w.Code, w.Body.String())
	}
	var cr GitCommitResponse
	_ = json.Unmarshal(w.Body.Bytes(), &cr)
	if !cr.Committed || cr.CommitID == "" || cr.Pushed {
		t.Fatalf("unexpected commit result: %+v", cr)
	}

	// 带 push 但没有 remote：提交成功，pushError 非空，HTTP 仍是 200 以便客户端提供重试
	w = postJSON(t, p.HandleGitCommit, "/gateway/git/commit", `{"cascadeId":"casc-1","message":"fix: a","push":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("commit+push: %d %s", w.Code, w.Body.String())
	}
	_ = json.Unmarshal(w.Body.Bytes(), &cr)
	if !cr.Committed || cr.Pushed || cr.PushError == "" {
		t.Fatalf("push failure must be reported separately: %+v", cr)
	}
	if st2, _ := gitStatus(context.Background(), root); !st2.Clean {
		t.Fatalf("tree should be clean after commits: %+v", st2.Files)
	}
}

func TestHandleGit_NotARepoAndPureChat(t *testing.T) {
	requireGit(t)
	t.Setenv("GIT_CEILING_DIRECTORIES", "/")
	plain := t.TempDir()
	p := fakeLSWithWorkspace(t, "casc-2", "file://"+plain)
	if w := postJSON(t, p.HandleGitStatus, "/x", `{"cascadeId":"casc-2"}`); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("non-repo => %d %s", w.Code, w.Body.String())
	}
	// 会话不存在 / 没有工作区（纯对话）
	if w := postJSON(t, p.HandleGitStatus, "/x", `{"cascadeId":"unknown"}`); w.Code == http.StatusOK {
		t.Errorf("unknown cascade must not succeed")
	}
}
