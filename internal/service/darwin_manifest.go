package service

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strings"
)

// DarwinPlistPath returns ~/Library/LaunchAgents/com.multigravity.gateway.plist.
func DarwinPlistPath(homeDir string) string {
	return filepath.Join(homeDir, "Library", "LaunchAgents", DefaultDarwinLabel+".plist")
}

func xmlEscape(s string) (string, error) {
	var buf bytes.Buffer
	if err := xml.EscapeText(&buf, []byte(s)); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// RenderDarwinPlist generates a macOS LaunchAgent plist for mgy.
func RenderDarwinPlist(cfg InstallConfig, homeDir string) (string, error) {
	norm, err := NormalizeInstallConfig(cfg)
	if err != nil {
		return "", err
	}
	// launchd natively redirects StandardOutPath & StandardErrorPath to LogPath.
	args, err := BuildServiceArgs(norm, false)
	if err != nil {
		return "", err
	}

	progArgs := make([]string, 0, 1+len(args))
	progArgs = append(progArgs, norm.BinaryPath)
	progArgs = append(progArgs, args...)

	var progXML strings.Builder
	for _, a := range progArgs {
		esc, err := xmlEscape(a)
		if err != nil {
			return "", err
		}
		progXML.WriteString("\t\t<string>")
		progXML.WriteString(esc)
		progXML.WriteString("</string>\n")
	}

	escLabel, _ := xmlEscape(DefaultDarwinLabel)
	escWorkDir, _ := xmlEscape(norm.DataDir)
	escLogPath, _ := xmlEscape(norm.LogPath)
	escHome, _ := xmlEscape(homeDir)

	pathParts := []string{"/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin", "/usr/sbin", "/sbin"}
	if strings.TrimSpace(homeDir) != "" {
		pathParts = append([]string{filepath.Join(homeDir, ".local", "bin")}, pathParts...)
	}
	escPath, _ := xmlEscape(strings.Join(pathParts, ":"))

	envSection := ""
	if strings.TrimSpace(homeDir) != "" {
		envSection = fmt.Sprintf(`	<key>EnvironmentVariables</key>
	<dict>
		<key>HOME</key>
		<string>%s</string>
		<key>PATH</key>
		<string>%s</string>
	</dict>
`, escHome, escPath)
	} else {
		envSection = fmt.Sprintf(`	<key>EnvironmentVariables</key>
	<dict>
		<key>PATH</key>
		<string>%s</string>
	</dict>
`, escPath)
	}

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
%s	</array>
	<key>WorkingDirectory</key>
	<string>%s</string>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>ThrottleInterval</key>
	<integer>5</integer>
	<key>ProcessType</key>
	<string>Background</string>
%s	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, escLabel, progXML.String(), escWorkDir, envSection, escLogPath, escLogPath)

	return plist, nil
}
