package proxy

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	approvalDiffMaxLines = 40
	approvalDiffMaxBytes = 2560
)

var (
	reDestructiveRM    = regexp.MustCompile(`(?i)(^|[;&|(\s])rm\s+-[a-zA-Z]*[rf][a-zA-Z]*\b|(^|[;&|(\s])git\s+clean\s+-[a-zA-Z]*f`)
	reForceGit         = regexp.MustCompile(`(?i)\bgit\s+(push\s+[^;&|]*(--force\b|--force-with-lease\b|-f\b)|reset\s+--hard\b|checkout\s+--\s+\.|branch\s+-D\b)`)
	reGitPush          = regexp.MustCompile(`(?i)\bgit\s+push\b`)
	rePrivilegeExec    = regexp.MustCompile(`(?i)(^|[;&|(\s])(sudo|doas)\s+|\bchmod\s+(-R\s+)?0?777\b|\bchown\s+-R\b`)
	reRemotePipeExec   = regexp.MustCompile(`(?i)\b(curl|wget)\b[^|;\n]*\|\s*(sudo\s+)?(sh|bash|zsh|python3?)\b`)
	reDestructiveSQL   = regexp.MustCompile(`(?i)\b(drop\s+(table|database|schema)|truncate\s+table)\b`)
	reDeleteFromSQL    = regexp.MustCompile(`(?i)\bdelete\s+from\s+[a-zA-Z0-9_."` + "`" + `]+`)
	reSystemDestruct  = regexp.MustCompile(`(?i)(^|[;&|(\s])(mkfs(\.[a-z0-9]+)?|dd\s+if=|shutdown|reboot|halt|killall\s+-9)\b`)
	reSensitiveTokens = regexp.MustCompile(`[\s"'=;|&()<>]+`)
	reHomePrefix      = regexp.MustCompile(`^(/Users/[^/]+|/home/[^/]+)(/.*)?$`)
)

// buildApprovalPreview constructs risk classification, shortened display path / cwd,
// and a bounded compact diff preview for an interactive approval step.
func buildApprovalPreview(pi *PendingInteraction, step TrajectoryStep, workspaceURIs []string) *ApprovalPreviewDetails {
	if pi == nil || pi.Type == "ask_question" || pi.Type == "elicitation" {
		return nil
	}

	preview := &ApprovalPreviewDetails{RiskLevel: "low"}
	reasonSet := make(map[string]bool)
	addReason := func(code string, level string) {
		if !reasonSet[code] {
			reasonSet[code] = true
			preview.RiskReasons = append(preview.RiskReasons, code)
		}
		if riskRank(level) > riskRank(preview.RiskLevel) {
			preview.RiskLevel = level
		}
	}

	wsPaths := normalizeWorkspacePaths(workspaceURIs)
	toolName := strings.ToLower(strings.TrimSpace(extractToolNameFromStep(step)))

	// 1. Extract structured file-edit / write diff preview from step tool args if present.
	rawArgs := extractStepToolArgsJSON(step)
	if rawArgs == "" && step.Generic != nil && len(step.Generic.Args) > 0 {
		if b, err := json.Marshal(step.Generic.Args); err == nil {
			rawArgs = string(b)
		}
	}
	editTargetPath := ""
	if rawArgs != "" {
		editPath, isOverwrite := populateEditDiffPreview(preview, toolName, rawArgs)
		if editPath != "" {
			editTargetPath = editPath
		}
		if isOverwrite {
			addReason("overwrite_file", "medium")
		}
	}

	// 2. Determine target path / command / cwd according to interaction type.
	cmdLine := ""
	rawTargetPath := editTargetPath
	switch pi.Type {
	case "run_command":
		cmdLine = strings.TrimSpace(pi.Target)
		if step.RunCommand != nil {
			if cmdLine == "" {
				cmdLine = strings.TrimSpace(step.RunCommand.CommandLine)
			}
			if cwd := strings.TrimSpace(step.RunCommand.Cwd); cwd != "" {
				preview.Cwd = formatCompactPath(cwd, wsPaths)
			}
		}
	case "file_permission":
		if rawTargetPath == "" {
			rawTargetPath = strings.TrimSpace(pi.Target)
		}
		addReason("outside_workspace", "medium")
	case "permission":
		actLower := strings.ToLower(pi.Action)
		if strings.Contains(actLower, "command") || strings.Contains(actLower, "exec") || strings.Contains(actLower, "shell") || strings.Contains(actLower, "terminal") {
			cmdLine = strings.TrimSpace(pi.Target)
		} else if rawTargetPath == "" {
			rawTargetPath = strings.TrimSpace(pi.Target)
		}
		if step.RunCommand != nil && strings.TrimSpace(step.RunCommand.Cwd) != "" {
			preview.Cwd = formatCompactPath(step.RunCommand.Cwd, wsPaths)
		}
	case "mcp":
		mcpLower := strings.ToLower(pi.Title + " " + toolName)
		for _, verb := range []string{"delete", "remove", "drop", "send", "post", "publish", "deploy", "push", "destroy", "revoke"} {
			if strings.Contains(mcpLower, verb) {
				addReason("external_write", "medium")
				break
			}
		}
	}

	// Fallback cwd extraction from run_command args JSON when step.RunCommand is absent.
	if preview.Cwd == "" && rawArgs != "" && (pi.Type == "run_command" || toolName == "run_command") {
		var cmdArgs struct {
			Cwd string `json:"Cwd"`
			Low string `json:"cwd"`
		}
		if json.Unmarshal([]byte(rawArgs), &cmdArgs) == nil {
			if c := firstNonEmptyStr(cmdArgs.Cwd, cmdArgs.Low); c != "" {
				preview.Cwd = formatCompactPath(c, wsPaths)
			}
		}
	}

	// 3. Classify command risks.
	if cmdLine != "" {
		if reDestructiveRM.MatchString(cmdLine) {
			addReason("destructive_delete", "high")
		}
		if reForceGit.MatchString(cmdLine) {
			addReason("force_git", "high")
		} else if reGitPush.MatchString(cmdLine) {
			addReason("git_push", "medium")
		}
		if rePrivilegeExec.MatchString(cmdLine) {
			addReason("privilege_exec", "high")
		}
		if reRemotePipeExec.MatchString(cmdLine) {
			addReason("remote_pipe_exec", "high")
		}
		if reDestructiveSQL.MatchString(cmdLine) || (reDeleteFromSQL.MatchString(cmdLine) && !strings.Contains(strings.ToLower(cmdLine), " where ")) {
			addReason("destructive_sql", "high")
		}
		if reSystemDestruct.MatchString(cmdLine) {
			addReason("system_ops", "high")
		}
		if hasSensitivePath(cmdLine) {
			addReason("sensitive_path", "high")
		}
	}

	// 4. Classify Git/GitHub permission grant risks.
	if pg := pi.PermissionGrant; pg != nil {
		actLower := strings.ToLower(pg.Action)
		branchLower := strings.ToLower(strings.TrimSpace(pg.Branch))
		if pg.Binary == "git" && strings.Contains(actLower, "push") {
			if strings.Contains(actLower, "force") || branchLower == "main" || branchLower == "master" || branchLower == "prod" || branchLower == "production" {
				addReason("force_git", "high")
			} else {
				addReason("git_push", "medium")
			}
		} else if pg.Binary == "gh" && (strings.Contains(actLower, "delete") || strings.Contains(actLower, "merge") || strings.Contains(actLower, "release")) {
			addReason("external_write", "medium")
		}
	}

	// 5. Classify file path sensitivity & workspace boundary.
	if rawTargetPath != "" {
		rawLower := strings.ToLower(strings.TrimSpace(rawTargetPath))
		if !strings.HasPrefix(rawLower, "http://") && !strings.HasPrefix(rawLower, "https://") {
			cleanPath := cleanFileURIOrPath(rawTargetPath)
			preview.DisplayPath = formatCompactPath(cleanPath, wsPaths)
			if hasSensitivePath(cleanPath) {
				addReason("sensitive_path", "high")
			}
			if filepath.IsAbs(cleanPath) && len(wsPaths) > 0 && !isInsideAnyWorkspace(cleanPath, wsPaths) {
				addReason("outside_workspace", "medium")
			}
		}
	}

	hasMeaningfulDisplayPath := preview.DisplayPath != "" && preview.DisplayPath != strings.TrimSpace(pi.Target)
	if preview.RiskLevel == "low" &&
		len(preview.RiskReasons) == 0 &&
		!hasMeaningfulDisplayPath &&
		preview.Cwd == "" &&
		preview.DiffPreview == "" &&
		preview.AddedLines == 0 &&
		preview.RemovedLines == 0 {
		return nil
	}
	return preview
}

func riskRank(level string) int {
	switch level {
	case "high":
		return 3
	case "medium":
		return 2
	default:
		return 1
	}
}

func hasSensitivePath(s string) bool {
	if s == "" {
		return false
	}
	tokens := reSensitiveTokens.Split(s, -1)
	for _, tok := range tokens {
		tok = strings.Trim(tok, "` ,:;()[]{}")
		if tok == "" {
			continue
		}
		tok = strings.ReplaceAll(tok, "\\", "/")
		segments := strings.Split(strings.ToLower(tok), "/")
		for _, seg := range segments {
			if seg == "" {
				continue
			}
			if seg == ".env" || strings.HasPrefix(seg, ".env.") {
				switch seg {
				case ".env.example", ".env.sample", ".env.template", ".env.dist", ".env.defaults":
					continue
				default:
					return true
				}
			}
			switch seg {
			case ".ssh", ".aws", ".gnupg", ".mgy-secrets",
				"id_rsa", "id_ed25519", "id_ecdsa", "id_dsa", "authorized_keys",
				"credentials", "credentials.json", ".netrc", ".npmrc", ".pypirc",
				"secrets.yaml", "secrets.yml", "secrets.json", "secrets.toml",
				"secret.yaml", "secret.yml", "secret.json", "secret.toml":
				return true
			}
			for _, ext := range []string{".pem", ".key", ".p12", ".pfx", ".jks", ".keystore"} {
				if len(seg) > len(ext) && strings.HasSuffix(seg, ext) {
					return true
				}
			}
		}
	}
	return false
}

func firstNonEmptyApprovalStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func cleanFileURIOrPath(raw string) string {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(s), "file://") {
		if u, err := url.Parse(s); err == nil && u.Path != "" {
			s = u.Path
		} else {
			s = strings.TrimPrefix(s, "file://")
		}
	}
	if decoded, err := url.PathUnescape(s); err == nil && decoded != "" {
		s = decoded
	}
	return s
}

func normalizeWorkspacePaths(uris []string) []string {
	out := make([]string, 0, len(uris))
	for _, u := range uris {
		p := strings.TrimRight(cleanFileURIOrPath(u), "/")
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func isInsideAnyWorkspace(path string, wsPaths []string) bool {
	clean := filepath.Clean(path)
	for _, ws := range wsPaths {
		wsClean := filepath.Clean(ws)
		if clean == wsClean || strings.HasPrefix(clean, wsClean+"/") {
			return true
		}
	}
	return false
}

func formatCompactPath(rawPath string, wsPaths []string) string {
	clean := cleanFileURIOrPath(rawPath)
	if clean == "" {
		return ""
	}
	for _, ws := range wsPaths {
		wsClean := strings.TrimRight(filepath.Clean(ws), "/")
		if clean == wsClean {
			return filepath.Base(wsClean)
		}
		if strings.HasPrefix(clean, wsClean+"/") {
			return strings.TrimPrefix(clean, wsClean+"/")
		}
	}
	if m := reHomePrefix.FindStringSubmatch(clean); len(m) >= 2 {
		if m[2] == "" {
			return "~"
		}
		return "~" + m[2]
	}
	return clean
}

type editChunkPair struct {
	oldText string
	newText string
}

func populateEditDiffPreview(preview *ApprovalPreviewDetails, toolName, rawArgs string) (targetPath string, isOverwrite bool) {
	var parsed struct {
		Path               string `json:"Path"`
		PathLow            string `json:"path"`
		TargetFile         string `json:"TargetFile"`
		TargetFileLow      string `json:"targetFile"`
		Target             string `json:"Target"`
		TargetLow          string `json:"target"`
		Replacement        string `json:"Replacement"`
		ReplacementLow     string `json:"replacement"`
		TargetContent      string `json:"TargetContent"`
		ReplacementContent string `json:"ReplacementContent"`
		Content            string `json:"Content"`
		ContentLow         string `json:"content"`
		CodeContent        string `json:"CodeContent"`
		Overwrite          bool   `json:"Overwrite"`
		OverwriteLow       bool   `json:"overwrite"`
		Append             bool   `json:"Append"`
		ReplacementChunks  []struct {
			TargetContent      string `json:"TargetContent"`
			ReplacementContent string `json:"ReplacementContent"`
		} `json:"ReplacementChunks"`
	}
	if err := json.Unmarshal([]byte(rawArgs), &parsed); err != nil {
		return "", false
	}

	targetPath = firstNonEmptyApprovalStr(parsed.Path, parsed.PathLow, parsed.TargetFile, parsed.TargetFileLow)
	overwrite := parsed.Overwrite || parsed.OverwriteLow

	var chunks []editChunkPair
	if len(parsed.ReplacementChunks) > 0 {
		for _, ch := range parsed.ReplacementChunks {
			if ch.TargetContent != "" || ch.ReplacementContent != "" {
				chunks = append(chunks, editChunkPair{oldText: ch.TargetContent, newText: ch.ReplacementContent})
			}
		}
	} else {
		oldStr := firstNonEmptyApprovalStr(parsed.Target, parsed.TargetLow, parsed.TargetContent)
		newStr := firstNonEmptyApprovalStr(parsed.Replacement, parsed.ReplacementLow, parsed.ReplacementContent)
		if oldStr != "" || newStr != "" {
			chunks = append(chunks, editChunkPair{oldText: oldStr, newText: newStr})
		} else {
			fullContent := firstNonEmptyApprovalStr(parsed.Content, parsed.ContentLow, parsed.CodeContent)
			if fullContent != "" && (targetPath != "" || strings.Contains(toolName, "write")) {
				preview.IsNewFile = !parsed.Append
				chunks = append(chunks, editChunkPair{oldText: "", newText: fullContent})
				isOverwrite = overwrite
			}
		}
	}

	if len(chunks) == 0 {
		return targetPath, isOverwrite
	}

	var diffLines []string
	totalAdded := 0
	totalRemoved := 0

	for cIdx, ch := range chunks {
		if cIdx > 0 {
			diffLines = append(diffLines, "@@")
		}
		oldLines := splitLinesNonEmpty(ch.oldText)
		newLines := splitLinesNonEmpty(ch.newText)

		// Trim identical leading & trailing lines to keep mobile preview ultra-compact.
		prefix := 0
		for prefix < len(oldLines) && prefix < len(newLines) && oldLines[prefix] == newLines[prefix] {
			prefix++
		}
		oldEnd := len(oldLines)
		newEnd := len(newLines)
		for oldEnd > prefix && newEnd > prefix && oldLines[oldEnd-1] == newLines[newEnd-1] {
			oldEnd--
			newEnd--
		}

		if prefix > 0 {
			diffLines = append(diffLines, "  "+oldLines[prefix-1])
		}
		for i := prefix; i < oldEnd; i++ {
			totalRemoved++
			diffLines = append(diffLines, "- "+oldLines[i])
		}
		for i := prefix; i < newEnd; i++ {
			totalAdded++
			diffLines = append(diffLines, "+ "+newLines[i])
		}
		if oldEnd < len(oldLines) {
			diffLines = append(diffLines, "  "+oldLines[oldEnd])
		}
	}

	preview.AddedLines = totalAdded
	preview.RemovedLines = totalRemoved

	if len(diffLines) == 0 {
		return targetPath, isOverwrite
	}

	var out strings.Builder
	truncated := false
	for idx, line := range diffLines {
		if idx >= approvalDiffMaxLines {
			truncated = true
			break
		}
		// Clip ultra-long minified single lines to protect mobile rendering.
		if len(line) > 220 {
			line = line[:217] + "..."
		}
		need := len(line)
		if idx > 0 {
			need++
		}
		if out.Len()+need > approvalDiffMaxBytes {
			truncated = true
			break
		}
		if idx > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(line)
	}

	preview.DiffPreview = out.String()
	preview.DiffTruncated = truncated
	return targetPath, isOverwrite
}

func splitLinesNonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}
