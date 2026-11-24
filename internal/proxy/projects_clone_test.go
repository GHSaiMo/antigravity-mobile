package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupCloneTestEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", home)
	t.Setenv("ANTIGRAVITY_APP_DATA_DIR", filepath.Join(home, ".gemini", "antigravity"))
	t.Setenv("MULTIGRAVITY_DATA_DIR", filepath.Join(home, "mgy-data"))
	t.Setenv("ALLOWED_WORKSPACE_ROOTS", "")
	t.Setenv("MULTIGRAVITY_ALLOWED_WORKSPACE_ROOTS", "")
	resetWorkspaceRootsForTest()
	t.Cleanup(resetWorkspaceRootsForTest)
	return evalSymlinksExistingPrefix(home)
}

func TestHandleListLocalDirsAndMkdir(t *testing.T) {
	home := setupCloneTestEnv(t)

	projectsDir := filepath.Join(home, "Projects")
	websitesDir := filepath.Join(home, "Websites")
	appA := filepath.Join(projectsDir, "alpha-app")
	appB := filepath.Join(projectsDir, "beta-app")
	siteA := filepath.Join(websitesDir, "landing")
	hiddenDir := filepath.Join(projectsDir, ".secret-dot")
	sshDir := filepath.Join(home, ".ssh")

	for _, d := range []string{appA, appB, siteA, hiddenDir, sshDir, filepath.Join(appA, ".git")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Register 2 projects in ~/Projects and 1 in ~/Websites so Projects ranks #1
	p := &Proxy{}
	for _, dir := range []string{appA, appB, siteA} {
		body, _ := json.Marshal(map[string]string{"path": dir})
		rec := httptest.NewRecorder()
		p.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/gateway/projects/create", bytes.NewReader(body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("seed project %s failed: %d %s", dir, rec.Code, rec.Body.String())
		}
	}

	// 1. Default GET /gateway/fs/dirs picks ~/Projects as defaultDir and lists subdirs
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/gateway/fs/dirs", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp LocalDirsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.CurrentPath != projectsDir || resp.DefaultDir != projectsDir {
		t.Fatalf("unexpected default dirs resp: current=%q default=%q want=%q", resp.CurrentPath, resp.DefaultDir, projectsDir)
	}
	if !resp.CanGoUp || resp.ParentPath != home {
		t.Fatalf("expected canGoUp=true parentPath=%q, got canGoUp=%v parentPath=%q", home, resp.CanGoUp, resp.ParentPath)
	}
	if len(resp.RecommendedDirs) < 3 {
		t.Fatalf("expected at least 3 recommendedDirs (Projects, Websites, ~), got %+v", resp.RecommendedDirs)
	}
	if resp.RecommendedDirs[0].Path != projectsDir || resp.RecommendedDirs[0].ProjectCount != 2 || !resp.RecommendedDirs[0].IsDefault {
		t.Fatalf("expected Projects ranked #1 with 2 projects, got %+v", resp.RecommendedDirs[0])
	}
	// Hidden dirs are filtered out by default; alpha-app should be marked as both git repo & project
	if len(resp.Entries) != 2 {
		t.Fatalf("expected 2 visible entries in Projects, got %+v", resp.Entries)
	}
	if resp.Entries[0].Name != "alpha-app" || !resp.Entries[0].IsGitRepo || !resp.Entries[0].IsProject {
		t.Fatalf("expected alpha-app with isGitRepo=true isProject=true, got %+v", resp.Entries[0])
	}

	// 2. Browsing home must NOT expose sensitive dirs like .ssh even with showHidden=true
	recHome := httptest.NewRecorder()
	p.ServeHTTP(recHome, httptest.NewRequest(http.MethodGet, "/gateway/fs/dirs?path=~&showHidden=true", nil))
	if recHome.Code != http.StatusOK {
		t.Fatalf("expected 200 for home, got %d: %s", recHome.Code, recHome.Body.String())
	}
	var homeResp LocalDirsResponse
	_ = json.Unmarshal(recHome.Body.Bytes(), &homeResp)
	for _, e := range homeResp.Entries {
		if e.Name == ".ssh" {
			t.Fatalf("sensitive directory .ssh must never be listed")
		}
	}

	// 3. POST /gateway/fs/mkdir creates a new directory under parentPath
	mkBody, _ := json.Marshal(map[string]string{
		"parentPath": projectsDir,
		"name":       "oss-sandbox",
	})
	recMk := httptest.NewRecorder()
	p.ServeHTTP(recMk, httptest.NewRequest(http.MethodPost, "/gateway/fs/mkdir", bytes.NewReader(mkBody)))
	if recMk.Code != http.StatusOK {
		t.Fatalf("expected 200 for mkdir, got %d: %s", recMk.Code, recMk.Body.String())
	}
	createdDir := filepath.Join(projectsDir, "oss-sandbox")
	if fi, err := os.Stat(createdDir); err != nil || !fi.IsDir() {
		t.Fatalf("expected directory %s to be created", createdDir)
	}

	// 4. Reject unsafe dir names & paths outside workspace roots
	for _, bad := range []string{
		`{"parentPath":"` + projectsDir + `","name":"../escape"}`,
		`{"parentPath":"` + projectsDir + `","name":"a/b"}`,
		`{"parentPath":"` + projectsDir + `","name":"-flag"}`,
		`{"path":"/etc/pwn"}`,
	} {
		recBad := httptest.NewRecorder()
		p.ServeHTTP(recBad, httptest.NewRequest(http.MethodPost, "/gateway/fs/mkdir", strings.NewReader(bad)))
		if recBad.Code == http.StatusOK {
			t.Fatalf("expected non-200 for bad mkdir %s, got 200: %s", bad, recBad.Body.String())
		}
	}
}

func TestHandleCloneGitHubRepo_AsyncRegistersProjectAndHandlesConflicts(t *testing.T) {
	home := setupCloneTestEnv(t)
	destParent := filepath.Join(home, "CustomRepos")
	if err := os.MkdirAll(destParent, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a real local git repo to simulate a clean git clone result
	var createPayloads []map[string]interface{}
	p := setupMockUpstreamProxy(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/ResolveFolder"):
			_, _ = w.Write([]byte(`{"resourceType":"FOLDER_TYPE_GIT_FOLDER"}`))
		case strings.HasSuffix(r.URL.Path, "/GetRepoInfos"):
			_, _ = w.Write([]byte(`{"repos":[{"branches":[{"name":"main","isCurrentBranch":true}]}]}`))
		case strings.HasSuffix(r.URL.Path, "/CreateProject"):
			var payload map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			createPayloads = append(createPayloads, payload)
			// Persist into ~/.gemini/config/projects just like real language_server
			proj := payload["project"].(map[string]interface{})
			resources := proj["projectResources"].(map[string]interface{})["resources"].([]interface{})
			_ = writeFallbackGeminiProjectFile(proj["id"].(string), proj["name"].(string), resources[0].(map[string]interface{}))
			_, _ = w.Write([]byte(`{}`))
		case strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories"):
			_, _ = w.Write([]byte(`{"trajectorySummaries":{}}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	})

	origRunner := localGitCloneRunner
	t.Cleanup(func() { localGitCloneRunner = origRunner })
	localGitCloneRunner = func(ctx context.Context, _ *Proxy, job *cloneRepoJob) (error, string) {
		job.updateProgress("cloning", 55, "正在拉取仓库数据 (50%)…")
		if err := os.MkdirAll(job.targetDir, 0o755); err != nil {
			return err, err.Error()
		}
		if err := exec.Command("git", "init", "-b", "main", job.targetDir).Run(); err != nil {
			// Fallback if git -b unsupported on older git
			if err2 := exec.Command("git", "init", job.targetDir).Run(); err2 != nil {
				return err2, err2.Error()
			}
		}
		_ = exec.Command("git", "-C", job.targetDir, "remote", "add", "origin", job.cloneURL).Run()
		if err := os.WriteFile(filepath.Join(job.targetDir, "README.md"), []byte("# demo\n"), 0o644); err != nil {
			return err, err.Error()
		}
		return nil, ""
	}

	// 1. Start async clone into CustomRepos
	reqBody, _ := json.Marshal(map[string]interface{}{
		"repoSlug":  "https://github.com/octocat/hello-world.git",
		"parentDir": destParent,
		"async":     true,
	})
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/gateway/projects/clone", bytes.NewReader(reqBody)))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 starting async clone, got %d: %s", rec.Code, rec.Body.String())
	}
	var startResp CloneJobStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &startResp); err != nil {
		t.Fatal(err)
	}
	if !startResp.OK || !startResp.Async || startResp.JobID == "" {
		t.Fatalf("unexpected async start response: %+v", startResp)
	}

	// 2. Poll until done
	var finalResp CloneJobStatusResponse
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		stRec := httptest.NewRecorder()
		p.ServeHTTP(stRec, httptest.NewRequest(http.MethodGet, "/gateway/projects/clone/status?jobId="+startResp.JobID, nil))
		if stRec.Code != http.StatusOK {
			t.Fatalf("status check failed: %d %s", stRec.Code, stRec.Body.String())
		}
		if err := json.Unmarshal(stRec.Body.Bytes(), &finalResp); err != nil {
			t.Fatal(err)
		}
		if finalResp.Status == "done" || finalResp.Status == "failed" || finalResp.Status == "cancelled" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	expectedTarget := filepath.Join(destParent, "hello-world")
	if finalResp.Status != "done" || finalResp.Project == nil || finalResp.Project.Path != expectedTarget {
		t.Fatalf("expected done with project at %s, got %+v", expectedTarget, finalResp)
	}
	if len(createPayloads) != 1 {
		t.Fatalf("expected CreateProject RPC to be called once, got %d", len(createPayloads))
	}

	// Verify it immediately shows up in GET /gateway/projects
	projects, err := p.GetProjects()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, prj := range projects {
		if prj.Path == expectedTarget && prj.Name == "hello-world" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("cloned repo %s not found in GetProjects(): %+v", expectedTarget, projects)
	}

	// 3. Re-cloning the same repo into the same parent detects already_cloned (409)
	recDup := httptest.NewRecorder()
	p.ServeHTTP(recDup, httptest.NewRequest(http.MethodPost, "/gateway/projects/clone", bytes.NewReader(reqBody)))
	if recDup.Code != http.StatusConflict {
		t.Fatalf("expected 409 already_cloned, got %d: %s", recDup.Code, recDup.Body.String())
	}
	var dupResp CloneJobStatusResponse
	_ = json.Unmarshal(recDup.Body.Bytes(), &dupResp)
	if dupResp.Code != "already_cloned" {
		t.Fatalf("expected code already_cloned, got %+v", dupResp)
	}

	// 4. Retrying with importExisting: true succeeds immediately and reuses project
	impBody, _ := json.Marshal(map[string]interface{}{
		"repoSlug":       "octocat/hello-world",
		"parentDir":      destParent,
		"async":          true,
		"importExisting": true,
	})
	recImp := httptest.NewRecorder()
	p.ServeHTTP(recImp, httptest.NewRequest(http.MethodPost, "/gateway/projects/clone", bytes.NewReader(impBody)))
	if recImp.Code != http.StatusOK {
		t.Fatalf("expected 200 on importExisting, got %d: %s", recImp.Code, recImp.Body.String())
	}
	var impResp CloneJobStatusResponse
	_ = json.Unmarshal(recImp.Body.Bytes(), &impResp)
	if !impResp.OK || impResp.Status != "done" || !impResp.AlreadyExists || impResp.Project == nil {
		t.Fatalf("unexpected importExisting resp: %+v", impResp)
	}

	// 5. Unrelated non-empty folder returns target_exists (409)
	unrelated := filepath.Join(destParent, "busy-folder")
	_ = os.MkdirAll(unrelated, 0o755)
	_ = os.WriteFile(filepath.Join(unrelated, "note.txt"), []byte("hi"), 0o644)
	conflictBody, _ := json.Marshal(map[string]interface{}{
		"repoSlug":   "octocat/hello-world",
		"parentDir":  destParent,
		"folderName": "busy-folder",
		"async":      true,
	})
	recConflict := httptest.NewRecorder()
	p.ServeHTTP(recConflict, httptest.NewRequest(http.MethodPost, "/gateway/projects/clone", bytes.NewReader(conflictBody)))
	if recConflict.Code != http.StatusConflict || !strings.Contains(recConflict.Body.String(), `"code":"target_exists"`) {
		t.Fatalf("expected 409 target_exists, got %d: %s", recConflict.Code, recConflict.Body.String())
	}
}

func TestHandleCloneGitHubRepo_FallbackToUpstreamRPCAndCancel(t *testing.T) {
	home := setupCloneTestEnv(t)
	destParent := filepath.Join(home, "FallbackTest")
	_ = os.MkdirAll(destParent, 0o755)

	upstreamCloned := false
	p := setupMockUpstreamProxy(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/CloneGitHubRepo"):
			upstreamCloned = true
			var req struct {
				RepoSlug     string `json:"repoSlug"`
				ParentDirURI string `json:"parentDirUri"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			outDir := filepath.Join(uriToFilePath(req.ParentDirURI), "private-repo")
			_ = os.MkdirAll(filepath.Join(outDir, ".git"), 0o755)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"folderUri": filePathToURI(outDir),
			})
		case strings.HasSuffix(r.URL.Path, "/ResolveFolder"):
			_, _ = w.Write([]byte(`{"resourceType":"FOLDER_TYPE_GIT_FOLDER"}`))
		case strings.HasSuffix(r.URL.Path, "/GetRepoInfos"):
			_, _ = w.Write([]byte(`{"repos":[{"branches":[{"name":"main","isCurrentBranch":true}]}]}`))
		case strings.HasSuffix(r.URL.Path, "/CreateProject"):
			_, _ = w.Write([]byte(`{}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	})

	origRunner := localGitCloneRunner
	t.Cleanup(func() { localGitCloneRunner = origRunner })
	localGitCloneRunner = func(ctx context.Context, _ *Proxy, _ *cloneRepoJob) (error, string) {
		return errors.New("exit status 128"), "fatal: could not read Username for 'https://github.com': terminal prompts disabled"
	}

	// Synchronous call should seamlessly fall back to upstream CloneGitHubRepo RPC and register project
	body, _ := json.Marshal(map[string]interface{}{
		"repoSlug":  "acme/private-repo",
		"parentDir": destParent,
	})
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/gateway/projects/clone", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 via upstream fallback, got %d: %s", rec.Code, rec.Body.String())
	}
	if !upstreamCloned {
		t.Fatalf("expected fallback to upstream CloneGitHubRepo RPC")
	}

	// Now test cancellation cleans up newly created partial targetDir
	started := make(chan struct{})
	localGitCloneRunner = func(ctx context.Context, _ *Proxy, job *cloneRepoJob) (error, string) {
		_ = os.MkdirAll(job.targetDir, 0o755)
		close(started)
		<-ctx.Done()
		return ctx.Err(), "cancelled"
	}
	cancelReqBody, _ := json.Marshal(map[string]interface{}{
		"repoSlug":  "acme/slow-repo",
		"parentDir": destParent,
		"async":     true,
	})
	recStart := httptest.NewRecorder()
	p.ServeHTTP(recStart, httptest.NewRequest(http.MethodPost, "/gateway/projects/clone", bytes.NewReader(cancelReqBody)))
	var st CloneJobStatusResponse
	_ = json.Unmarshal(recStart.Body.Bytes(), &st)
	<-started

	recCancel := httptest.NewRecorder()
	cBody, _ := json.Marshal(map[string]string{"jobId": st.JobID})
	p.ServeHTTP(recCancel, httptest.NewRequest(http.MethodPost, "/gateway/projects/clone/cancel", bytes.NewReader(cBody)))
	if recCancel.Code != http.StatusOK {
		t.Fatalf("expected 200 on cancel, got %d: %s", recCancel.Code, recCancel.Body.String())
	}

	// Wait briefly for cleanup goroutine to finish
	deadline := time.Now().Add(2 * time.Second)
	slowTarget := filepath.Join(destParent, "slow-repo")
	for time.Now().Before(deadline) {
		if _, err := os.Stat(slowTarget); os.IsNotExist(err) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(slowTarget); !os.IsNotExist(err) {
		t.Fatalf("expected cancelled partial dir %s to be cleaned up", slowTarget)
	}
}
