package service

import (
	"fmt"
	"path/filepath"
	"strings"
)

// LinuxUnitPath returns ~/.config/systemd/user/mgy.service.
func LinuxUnitPath(homeDir string) string {
	return filepath.Join(homeDir, ".config", "systemd", "user", DefaultLinuxUnit)
}

func quoteSystemdArg(s string) string {
	// Escape backslashes, double quotes, and systemd specifiers (%) safely.
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `%`, `%%`)
	escaped := r.Replace(s)
	if escaped == "" || strings.ContainsAny(escaped, " \t'\"\\$") {
		return `"` + escaped + `"`
	}
	return escaped
}

func escapeSystemdValue(s string) string {
	return strings.ReplaceAll(s, "%", "%%")
}

// RenderLinuxUnit generates a systemd --user service unit for mgy.
func RenderLinuxUnit(cfg InstallConfig, homeDir string) (string, error) {
	norm, err := NormalizeInstallConfig(cfg)
	if err != nil {
		return "", err
	}
	// systemd natively appends stdout and stderr to LogPath.
	args, err := BuildServiceArgs(norm, false)
	if err != nil {
		return "", err
	}

	tokens := make([]string, 0, 1+len(args))
	tokens = append(tokens, quoteSystemdArg(norm.BinaryPath))
	for _, a := range args {
		tokens = append(tokens, quoteSystemdArg(a))
	}

	pathParts := []string{"/usr/local/bin", "/usr/bin", "/bin"}
	if strings.TrimSpace(homeDir) != "" {
		pathParts = append([]string{filepath.Join(homeDir, ".local", "bin")}, pathParts...)
	}

	unit := fmt.Sprintf(`[Unit]
Description=Multigravity (mgy) Gateway Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s
Restart=always
RestartSec=5
Environment=%s
StandardOutput=append:%s
StandardError=append:%s

[Install]
WantedBy=default.target
`,
		escapeSystemdValue(norm.DataDir),
		strings.Join(tokens, " "),
		quoteSystemdArg("PATH="+strings.Join(pathParts, ":")),
		escapeSystemdValue(norm.LogPath),
		escapeSystemdValue(norm.LogPath),
	)

	return unit, nil
}
