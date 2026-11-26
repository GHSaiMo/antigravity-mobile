package service

import (
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"strings"
	"unicode/utf16"
)

// WindowsTaskSpec controls Windows Task Scheduler principal and launch parameters.
type WindowsTaskSpec struct {
	UserID         string // e.g. "DESKTOP\\Alice"
	LogonOnly      bool   // true => InteractiveToken + LogonTrigger (+ conhost --headless); false => S4U + BootTrigger + LogonTrigger
	ConhostPath    string // optional C:\Windows\System32\conhost.exe for headless InteractiveToken launch
	LeastPrivilege bool   // false => HighestAvailable (recommended to avoid UAC elevation isolation)
}

// QuoteWindowsArg quotes an argument following Windows CommandLineToArgvW rules.
func QuoteWindowsArg(arg string) string {
	if arg == "" {
		return `""`
	}
	if !strings.ContainsAny(arg, " \t\"") {
		return arg
	}
	var sb strings.Builder
	sb.WriteByte('"')
	backslashes := 0
	for i := 0; i < len(arg); i++ {
		c := arg[i]
		if c == '\\' {
			backslashes++
			continue
		}
		if c == '"' {
			for j := 0; j < backslashes*2+1; j++ {
				sb.WriteByte('\\')
			}
			sb.WriteByte('"')
			backslashes = 0
			continue
		}
		for j := 0; j < backslashes; j++ {
			sb.WriteByte('\\')
		}
		backslashes = 0
		sb.WriteByte(c)
	}
	for j := 0; j < backslashes*2; j++ {
		sb.WriteByte('\\')
	}
	sb.WriteByte('"')
	return sb.String()
}

// FormatWindowsArgs joins argv tokens into a single CommandLineToArgvW-safe argument string.
func FormatWindowsArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = QuoteWindowsArg(a)
	}
	return strings.Join(quoted, " ")
}

// RenderWindowsTaskXML builds a Task Scheduler 1.2 XML definition for schtasks /Create /XML.
func RenderWindowsTaskXML(cfg InstallConfig, spec WindowsTaskSpec) (string, error) {
	norm, err := NormalizeInstallConfig(cfg)
	if err != nil {
		return "", err
	}
	userID := strings.TrimSpace(spec.UserID)
	if userID == "" {
		return "", fmt.Errorf("windows user ID is required")
	}
	if containsUnsafeControl(userID) {
		return "", fmt.Errorf("windows user ID contains invalid control characters")
	}

	// Pass -log-file directly to mgy.exe so Task Scheduler can run mgy.exe with no shell wrapper
	// while still persisting rotating UTF-8 logs to ~/.multigravity/logs/service.log.
	svcArgs, err := BuildServiceArgs(norm, true)
	if err != nil {
		return "", err
	}

	execCmd := norm.BinaryPath
	execArgs := FormatWindowsArgs(svcArgs)
	if spec.LogonOnly && strings.TrimSpace(spec.ConhostPath) != "" {
		execCmd = strings.TrimSpace(spec.ConhostPath)
		wrapped := make([]string, 0, 2+len(svcArgs))
		wrapped = append(wrapped, "--headless", norm.BinaryPath)
		wrapped = append(wrapped, svcArgs...)
		execArgs = FormatWindowsArgs(wrapped)
	}

	logonType := "S4U"
	if spec.LogonOnly {
		logonType = "InteractiveToken"
	}
	runLevel := "HighestAvailable"
	if spec.LeastPrivilege {
		runLevel = "LeastPrivilege"
	}

	escUser, err := xmlEscape(userID)
	if err != nil {
		return "", err
	}
	escCmd, err := xmlEscape(execCmd)
	if err != nil {
		return "", err
	}
	escArgs, err := xmlEscape(execArgs)
	if err != nil {
		return "", err
	}
	escWorkDir, err := xmlEscape(norm.DataDir)
	if err != nil {
		return "", err
	}

	var triggers strings.Builder
	if !spec.LogonOnly {
		triggers.WriteString("    <BootTrigger>\n      <Enabled>true</Enabled>\n    </BootTrigger>\n")
	}
	triggers.WriteString("    <LogonTrigger>\n      <Enabled>true</Enabled>\n      <UserId>")
	triggers.WriteString(escUser)
	triggers.WriteString("</UserId>\n    </LogonTrigger>\n")

	xmlDoc := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>Multigravity (mgy) Gateway Auto-Start Background Service</Description>
  </RegistrationInfo>
  <Triggers>
%s  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>%s</UserId>
      <LogonType>%s</LogonType>
      <RunLevel>%s</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <IdleSettings>
      <StopOnIdleEnd>false</StopOnIdleEnd>
      <RestartOnIdle>false</RestartOnIdle>
    </IdleSettings>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>true</Hidden>
    <RunOnlyIfIdle>false</RunOnlyIfIdle>
    <WakeToRun>false</WakeToRun>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>7</Priority>
    <RestartOnFailure>
      <Interval>PT1M</Interval>
      <Count>999</Count>
    </RestartOnFailure>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>%s</Command>
      <Arguments>%s</Arguments>
      <WorkingDirectory>%s</WorkingDirectory>
    </Exec>
  </Actions>
</Task>
`, triggers.String(), escUser, logonType, runLevel, escCmd, escArgs, escWorkDir)

	return xmlDoc, nil
}

// EncodeUTF16LEWithBOM converts a UTF-8 string to UTF-16 LE with BOM and CRLF line endings,
// matching the exact format expected by Windows schtasks.exe /Create /XML across all locales.
func EncodeUTF16LEWithBOM(s string) []byte {
	normalized := strings.ReplaceAll(s, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\n", "\r\n")
	units := utf16.Encode([]rune(normalized))
	out := make([]byte, 2+len(units)*2)
	out[0] = 0xFF
	out[1] = 0xFE
	for i, u := range units {
		binary.LittleEndian.PutUint16(out[2+i*2:], u)
	}
	return out
}

// WindowsTaskMetadata holds locale-independent fields parsed from schtasks /Query /XML output.
type WindowsTaskMetadata struct {
	Command          string
	Arguments        string
	WorkingDirectory string
	LogonType        string
	RunLevel         string
	HasBootTrigger   bool
	HasLogonTrigger  bool
}

// DecodeUTF16OrUTF8 decodes UTF-16 LE/BE (with BOM) or UTF-8 bytes into a Go string.
func DecodeUTF16OrUTF8(raw []byte) string {
	if len(raw) >= 2 && raw[0] == 0xFF && raw[1] == 0xFE {
		body := raw[2:]
		units := make([]uint16, len(body)/2)
		for i := range units {
			units[i] = binary.LittleEndian.Uint16(body[i*2:])
		}
		return string(utf16.Decode(units))
	}
	if len(raw) >= 2 && raw[0] == 0xFE && raw[1] == 0xFF {
		body := raw[2:]
		units := make([]uint16, len(body)/2)
		for i := range units {
			units[i] = binary.BigEndian.Uint16(body[i*2:])
		}
		return string(utf16.Decode(units))
	}
	// Strip UTF-8 BOM if present.
	if len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF {
		raw = raw[3:]
	}
	return string(raw)
}

// ParseWindowsTaskXMLMetadata parses Task Scheduler XML returned by schtasks /Query /XML.
func ParseWindowsTaskXMLMetadata(raw []byte) (WindowsTaskMetadata, error) {
	text := DecodeUTF16OrUTF8(raw)
	// encoding/xml rejects encoding="UTF-16" when fed UTF-8 string bytes; normalize header before decoding.
	text = strings.Replace(text, `encoding="UTF-16"`, `encoding="UTF-8"`, 1)
	text = strings.Replace(text, `encoding="utf-16"`, `encoding="UTF-8"`, 1)

	var doc struct {
		Triggers struct {
			BootTrigger  []struct{} `xml:"BootTrigger"`
			LogonTrigger []struct{} `xml:"LogonTrigger"`
		} `xml:"Triggers"`
		Principals struct {
			Principal []struct {
				LogonType string `xml:"LogonType"`
				RunLevel  string `xml:"RunLevel"`
			} `xml:"Principal"`
		} `xml:"Principals"`
		Actions struct {
			Exec []struct {
				Command          string `xml:"Command"`
				Arguments        string `xml:"Arguments"`
				WorkingDirectory string `xml:"WorkingDirectory"`
			} `xml:"Exec"`
		} `xml:"Actions"`
	}
	if err := xml.Unmarshal([]byte(text), &doc); err != nil {
		return WindowsTaskMetadata{}, err
	}
	var meta WindowsTaskMetadata
	meta.HasBootTrigger = len(doc.Triggers.BootTrigger) > 0
	meta.HasLogonTrigger = len(doc.Triggers.LogonTrigger) > 0
	if len(doc.Principals.Principal) > 0 {
		meta.LogonType = strings.TrimSpace(doc.Principals.Principal[0].LogonType)
		meta.RunLevel = strings.TrimSpace(doc.Principals.Principal[0].RunLevel)
	}
	if len(doc.Actions.Exec) > 0 {
		meta.Command = strings.TrimSpace(doc.Actions.Exec[0].Command)
		meta.Arguments = strings.TrimSpace(doc.Actions.Exec[0].Arguments)
		meta.WorkingDirectory = strings.TrimSpace(doc.Actions.Exec[0].WorkingDirectory)
	}
	return meta, nil
}
