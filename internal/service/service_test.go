package service

import (
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildServiceArgsModesAndFlags(t *testing.T) {
	cfg := InstallConfig{
		BinaryPath:  "/usr/local/bin/mgy",
		DataDir:     "/tmp/mgy-data",
		LogPath:     "/tmp/mgy-data/logs/service.log",
		Mode:        "hub",
		Port:        58910,
		Host:        "127.0.0.1",
		Headless:    true,
		Name:        "HomeHub",
		NoDiscovery: true,
	}

	args, err := BuildServiceArgs(cfg, true)
	if err != nil {
		t.Fatalf("BuildServiceArgs: %v", err)
	}
	got := strings.Join(args, " ")
	want := "hub -qr=false -port 58910 -host 127.0.0.1 -headless -name HomeHub -no-discovery -log-file /tmp/mgy-data/logs/service.log"
	if got != want {
		t.Fatalf("args = %q, want %q", got, want)
	}

	// Default mode omits optional flags so ~/.multigravity/.env stays dynamic.
	minCfg := InstallConfig{
		BinaryPath: "/usr/local/bin/mgy",
		DataDir:    "/tmp/mgy-data",
	}
	minArgs, err := BuildServiceArgs(minCfg, false)
	if err != nil {
		t.Fatalf("BuildServiceArgs minimal: %v", err)
	}
	if strings.Join(minArgs, " ") != "run -qr=false" {
		t.Fatalf("minArgs = %q, want %q", strings.Join(minArgs, " "), "run -qr=false")
	}

	// Reject invalid mode and control characters.
	bad := minCfg
	bad.Mode = "bogus"
	if _, err := BuildServiceArgs(bad, false); err == nil {
		t.Fatalf("expected error for invalid mode")
	}
	bad2 := minCfg
	bad2.Host = "127.0.0.1\nrm -rf /"
	if _, err := BuildServiceArgs(bad2, false); err == nil {
		t.Fatalf("expected error for newline in host")
	}
}

func TestRenderDarwinPlistValidXML(t *testing.T) {
	cfg := InstallConfig{
		BinaryPath: "/Users/alice/.local/bin/mgy",
		DataDir:    "/Users/alice/.multigravity",
		LogPath:    "/Users/alice/.multigravity/logs/service.log",
		Mode:       "run",
	}
	plist, err := RenderDarwinPlist(cfg, "/Users/alice")
	if err != nil {
		t.Fatalf("RenderDarwinPlist: %v", err)
	}

	// Must parse cleanly as XML.
	var v any
	if err := xml.Unmarshal([]byte(plist), &v); err != nil {
		t.Fatalf("invalid darwin plist XML: %v\n%s", err, plist)
	}
	for _, want := range []string{
		"<string>com.multigravity.gateway</string>",
		"<string>/Users/alice/.local/bin/mgy</string>",
		"<string>run</string>",
		"<string>-qr=false</string>",
		"<key>RunAtLoad</key>",
		"<key>KeepAlive</key>",
		"<string>/Users/alice/.multigravity/logs/service.log</string>",
	} {
		if !strings.Contains(plist, want) {
			t.Fatalf("plist missing %q:\n%s", want, plist)
		}
	}
	// macOS LaunchAgent uses StandardOutPath/StandardErrorPath, so -log-file should NOT be duplicated.
	if strings.Contains(plist, "-log-file") {
		t.Fatalf("darwin plist should not include -log-file to avoid duplicate logging")
	}
}

func TestRenderLinuxUnit(t *testing.T) {
	cfg := InstallConfig{
		BinaryPath: "/home/alice/.local/bin/mgy",
		DataDir:    "/home/alice/.multigravity",
		LogPath:    "/home/alice/.multigravity/logs/service.log",
		Mode:       "node",
		Name:       "Build Box",
		HubURL:     "http://192.168.50.10:58900",
	}
	unit, err := RenderLinuxUnit(cfg, "/home/alice")
	if err != nil {
		t.Fatalf("RenderLinuxUnit: %v", err)
	}
	for _, want := range []string{
		"WorkingDirectory=/home/alice/.multigravity",
		`ExecStart=/home/alice/.local/bin/mgy node -qr=false -name "Build Box" -hub http://192.168.50.10:58900`,
		"Restart=always",
		"StandardOutput=append:/home/alice/.multigravity/logs/service.log",
		"WantedBy=default.target",
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("linux unit missing %q:\n%s", want, unit)
		}
	}
}

func TestRenderWindowsTaskXMLAndUTF16RoundTrip(t *testing.T) {
	cfg := InstallConfig{
		BinaryPath: `C:\Users\Alice Smith\.local\bin\mgy.exe`,
		DataDir:    `C:\Users\Alice Smith\.multigravity`,
		LogPath:    `C:\Users\Alice Smith\.multigravity\logs\service.log`,
		Mode:       "run",
	}

	// Default S4U boot + logon mode.
	xmlDoc, err := RenderWindowsTaskXML(cfg, WindowsTaskSpec{
		UserID:    `DESKTOP-1\Alice Smith`,
		LogonOnly: false,
	})
	if err != nil {
		t.Fatalf("RenderWindowsTaskXML (S4U): %v", err)
	}
	encoded := EncodeUTF16LEWithBOM(xmlDoc)
	if len(encoded) < 2 || encoded[0] != 0xFF || encoded[1] != 0xFE {
		t.Fatalf("expected UTF-16 LE BOM")
	}

	meta, err := ParseWindowsTaskXMLMetadata(encoded)
	if err != nil {
		t.Fatalf("ParseWindowsTaskXMLMetadata: %v", err)
	}
	if meta.LogonType != "S4U" {
		t.Fatalf("LogonType = %q, want S4U", meta.LogonType)
	}
	if meta.RunLevel != "HighestAvailable" {
		t.Fatalf("RunLevel = %q, want HighestAvailable", meta.RunLevel)
	}
	if !meta.HasBootTrigger || !meta.HasLogonTrigger {
		t.Fatalf("expected both BootTrigger and LogonTrigger, got boot=%v logon=%v", meta.HasBootTrigger, meta.HasLogonTrigger)
	}
	if meta.Command != cfg.BinaryPath {
		t.Fatalf("Command = %q, want %q", meta.Command, cfg.BinaryPath)
	}
	if !strings.Contains(meta.Arguments, `-log-file "C:\Users\Alice Smith\.multigravity\logs\service.log"`) {
		t.Fatalf("Arguments missing quoted -log-file: %q", meta.Arguments)
	}
	for _, want := range []string{
		"<ExecutionTimeLimit>PT0S</ExecutionTimeLimit>",
		"<DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>",
		"<StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>",
		"<MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>",
		"<Interval>PT1M</Interval>",
		"<Count>999</Count>",
		"<Hidden>true</Hidden>",
	} {
		if !strings.Contains(xmlDoc, want) {
			t.Fatalf("windows XML missing %q:\n%s", want, xmlDoc)
		}
	}

	// Interactive logon-only fallback with conhost --headless.
	xmlLogon, err := RenderWindowsTaskXML(cfg, WindowsTaskSpec{
		UserID:      `DESKTOP-1\Alice Smith`,
		LogonOnly:   true,
		ConhostPath: `C:\Windows\System32\conhost.exe`,
	})
	if err != nil {
		t.Fatalf("RenderWindowsTaskXML (LogonOnly): %v", err)
	}
	metaLogon, err := ParseWindowsTaskXMLMetadata([]byte(xmlLogon))
	if err != nil {
		t.Fatalf("ParseWindowsTaskXMLMetadata (LogonOnly): %v", err)
	}
	if metaLogon.LogonType != "InteractiveToken" || metaLogon.HasBootTrigger || !metaLogon.HasLogonTrigger {
		t.Fatalf("unexpected LogonOnly metadata: %+v", metaLogon)
	}
	if metaLogon.Command != `C:\Windows\System32\conhost.exe` {
		t.Fatalf("Command = %q, want conhost.exe", metaLogon.Command)
	}
	if !strings.HasPrefix(metaLogon.Arguments, `--headless "C:\Users\Alice Smith\.local\bin\mgy.exe" run -qr=false`) {
		t.Fatalf("unexpected conhost Arguments: %q", metaLogon.Arguments)
	}
}

func TestQuoteWindowsArg(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"plain", "plain"},
		{"", `""`},
		{`C:\Users\My Name\mgy.exe`, `"C:\Users\My Name\mgy.exe"`},
		{`C:\dir with space\`, `"C:\dir with space\\"`},
		{`say "hi"`, `"say \"hi\""`},
	}
	for _, tc := range cases {
		if got := QuoteWindowsArg(tc.in); got != tc.want {
			t.Errorf("QuoteWindowsArg(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTailAndFollowLogs(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "service.log")
	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(logPath, []byte(content), 0600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	got, err := TailLogs(logPath, 3)
	if err != nil {
		t.Fatalf("TailLogs: %v", err)
	}
	if strings.Join(got, ",") != "line3,line4,line5" {
		t.Fatalf("TailLogs = %v, want [line3 line4 line5]", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var buf strings.Builder
	if err := FollowLogs(ctx, logPath, 2, &buf); err != nil {
		t.Fatalf("FollowLogs: %v", err)
	}
	if buf.String() != "line4\nline5\n" {
		t.Fatalf("FollowLogs initial output = %q, want %q", buf.String(), "line4\nline5\n")
	}
}
