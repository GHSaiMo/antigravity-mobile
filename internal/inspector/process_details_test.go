package inspector

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestVersionFromCommandLine(t *testing.T) {
	cases := map[string]string{
		"/Applications/Antigravity.app/Contents/Resources/bin/language_server --standalone --override_ide_name antigravity --override_ide_version 2.22.0 --override_user_agent_name antigravity": "2.22.0",
		"language_server --override_ide_version=2.30.1-beta.2 --x": "2.30.1-beta.2",
		"language_server --override_ide_version 2.22.0":            "2.22.0",
		"language_server --standalone":                             "",
		"language_server --override_ide_version":                   "",
		"":                                                         "",
	}
	for in, want := range cases {
		if got := VersionFromCommandLine(in); got != want {
			t.Errorf("VersionFromCommandLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLookupProcessDetailsInvalidPID(t *testing.T) {
	for _, pid := range []int{0, -1} {
		if d := LookupProcessDetails(pid); d != (ProcessDetails{}) {
			t.Errorf("pid %d => %+v", pid, d)
		}
	}
	// 不存在的进程：不报错，只是什么都拿不到
	if d := LookupProcessDetails(2147483000); d.ExecutablePath != "" || d.Version != "" {
		t.Errorf("nonexistent pid => %+v", d)
	}
}

func TestLookupProcessDetailsSelf(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("covered by the windows implementation")
	}
	self, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	selfReal, _ := filepath.EvalSymlinks(self)
	d := LookupProcessDetails(os.Getpid())
	if d.ExecutablePath == "" {
		t.Fatal("own executable path should be resolvable")
	}
	gotReal, _ := filepath.EvalSymlinks(d.ExecutablePath)
	if gotReal != selfReal {
		t.Errorf("path = %q, want %q", gotReal, selfReal)
	}
}

func TestLookupProcessDetailsReadsVersionFromCommandLine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix-only fixture")
	}
	cmd := exec.Command("sh", "-c", "sleep 20; true", "--override_ide_version", "9.9.9")
	if err := cmd.Start(); err != nil {
		t.Skip(err)
	}
	defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()
	time.Sleep(200 * time.Millisecond)

	d := LookupProcessDetails(cmd.Process.Pid)
	if d.Version != "9.9.9" {
		t.Errorf("version = %q, want 9.9.9 (details %+v)", d.Version, d)
	}
}
