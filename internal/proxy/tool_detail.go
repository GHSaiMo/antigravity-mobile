package proxy

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	maxToolTerminalOutputBytes = 4096
	maxToolTerminalOutputLines = 80
	maxToolMarkdownBytes       = 4096
)

var genericCommandNoiseRe = regexp.MustCompile(`(?i)^(running|run|execute|executing)?\s*(a\s*)?(shell\s*|bash\s*|terminal\s*)?command(s)?$`)

// isGenericCommandNoise mirrors Antigravity desktop's Cg() filter so placeholder labels
// like "Run command" fall back to the actual command line instead of hiding it.
func isGenericCommandNoise(s string) bool {
	trimmed := strings.TrimSpace(s)
	return trimmed == "" || genericCommandNoiseRe.MatchString(trimmed)
}

func isStepActiveStatus(status string) bool {
	return status == "CORTEX_STEP_STATUS_RUNNING" ||
		status == "CORTEX_STEP_STATUS_PENDING" ||
		status == "CORTEX_STEP_STATUS_GENERATING"
}

func stepDetailStatusLabel(s TrajectoryStep) string {
	if s.RunCommand != nil && s.RunCommand.UserRejected {
		return "rejected"
	}
	switch s.Status {
	case "CORTEX_STEP_STATUS_RUNNING", "CORTEX_STEP_STATUS_PENDING", "CORTEX_STEP_STATUS_GENERATING":
		return "running"
	case "CORTEX_STEP_STATUS_WAITING":
		return "waiting"
	case "CORTEX_STEP_STATUS_CANCELED":
		return "canceled"
	case "CORTEX_STEP_STATUS_INTERRUPTED":
		return "interrupted"
	case "CORTEX_STEP_STATUS_ERROR":
		return "error"
	default:
		if hasStepError(s) {
			return "error"
		}
		return "done"
	}
}

func stepErrorShortText(s TrajectoryStep) string {
	if s.ErrorMessage != nil {
		if msg := strings.TrimSpace(s.ErrorMessage.Error.UserErrorMessage); msg != "" {
			return msg
		}
		if msg := strings.TrimSpace(s.ErrorMessage.Error.ShortError); msg != "" {
			return msg
		}
	}
	if s.Error != nil {
		if msg := strings.TrimSpace(s.Error.UserErrorMessage); msg != "" {
			return msg
		}
		if msg := strings.TrimSpace(s.Error.ShortError); msg != "" {
			return msg
		}
		if msg := strings.TrimSpace(s.Error.FullError); msg != "" {
			if len(msg) > 280 {
				return msg[:277] + "..."
			}
			return msg
		}
	}
	return ""
}

func isHiddenToolStep(s TrajectoryStep) bool {
	switch s.Type {
	case "CORTEX_STEP_TYPE_CHECKPOINT", "CORTEX_STEP_TYPE_EPHEMERAL_MESSAGE":
		return true
	}
	if s.Generic != nil && s.Generic.Result != nil && s.Generic.Result.StepRenderInfo != nil {
		if s.Generic.Result.StepRenderInfo.Hidden {
			return true
		}
	}
	return false
}

func cleanFileURIToPath(raw string) string {
	s := strings.TrimSpace(strings.Trim(raw, `"`))
	if s == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(s), "file://") {
		if u, err := url.Parse(s); err == nil && u.Path != "" {
			if dec, err := url.PathUnescape(u.Path); err == nil && dec != "" {
				return dec
			}
			return u.Path
		}
		return strings.TrimPrefix(s, "file://")
	}
	if i := strings.IndexAny(s, "#?"); i >= 0 {
		s = s[:i]
	}
	return s
}

func normalizeFileURI(raw string) string {
	s := strings.TrimSpace(strings.Trim(raw, `"`))
	if s == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(s), "file://") {
		return s
	}
	if strings.HasPrefix(s, "/") {
		return "file://" + s
	}
	return s
}

func basenameOfFileURI(raw string) string {
	p := cleanFileURIToPath(raw)
	p = strings.TrimRight(p, `/\`)
	if p == "" {
		return ""
	}
	base := filepath.Base(p)
	if base == "." || base == "/" {
		return p
	}
	return base
}

func shortenCwd(raw string) string {
	p := cleanFileURIToPath(raw)
	p = strings.TrimRight(p, `/\`)
	if p == "" {
		return ""
	}
	return filepath.Base(p)
}

// truncateTailOutput bounds terminal output to the tail maxLines / maxBytes for mobile bandwidth
// and returns the count of omitted lines above the retained tail.
func truncateTailOutput(raw string, existingLinesAbove int, maxLines int, maxBytes int) (string, int) {
	if raw == "" {
		return "", existingLinesAbove
	}
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	extraAbove := 0
	if maxLines > 0 && len(lines) > maxLines {
		extraAbove = len(lines) - maxLines
		lines = lines[extraAbove:]
	}
	out := strings.Join(lines, "\n")
	if maxBytes > 0 && len(out) > maxBytes {
		cut := len(out) - maxBytes
		// Advance to next newline so we don't slice mid-UTF8 or mid-line when possible.
		if nl := strings.IndexByte(out[cut:], '\n'); nl >= 0 && cut+nl+1 < len(out) {
			extraAbove += strings.Count(out[:cut+nl+1], "\n")
			out = out[cut+nl+1:]
		} else {
			for cut < len(out) && (out[cut]&0xC0) == 0x80 {
				cut++
			}
			out = out[cut:]
		}
	}
	return out, existingLinesAbove + extraAbove
}

func truncateMarkdown(raw string, maxBytes int) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || maxBytes <= 0 || len(trimmed) <= maxBytes {
		return trimmed
	}
	cut := maxBytes
	for cut > 0 && (trimmed[cut]&0xC0) == 0x80 {
		cut--
	}
	return strings.TrimSpace(trimmed[:cut]) + "\n…"
}

func parseToolArgsMap(s TrajectoryStep) map[string]interface{} {
	out := make(map[string]interface{})
	if s.Generic != nil && len(s.Generic.Args) > 0 {
		for k, raw := range s.Generic.Args {
			var v interface{}
			if err := json.Unmarshal(raw, &v); err == nil {
				out[k] = v
			} else {
				out[k] = strings.Trim(strings.TrimSpace(string(raw)), `"`)
			}
		}
	}
	if rawJSON := strings.TrimSpace(extractStepToolArgsJSON(s)); rawJSON != "" && strings.HasPrefix(rawJSON, "{") {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(rawJSON), &parsed); err == nil {
			for k, v := range parsed {
				if _, exists := out[k]; !exists {
					out[k] = v
				}
			}
		}
	}
	return out
}

func argString(args map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := args[k]; ok && v != nil {
			if str, ok := v.(string); ok {
				if trimmed := strings.TrimSpace(str); trimmed != "" {
					return trimmed
				}
			}
		}
	}
	return ""
}

func argInt(args map[string]interface{}, keys ...string) int {
	for _, k := range keys {
		v, ok := args[k]
		if !ok || v == nil {
			continue
		}
		switch n := v.(type) {
		case float64:
			if n > 0 {
				return int(n)
			}
		case int:
			if n > 0 {
				return n
			}
		case string:
			if parsed, err := strconv.Atoi(strings.TrimSpace(n)); err == nil && parsed > 0 {
				return parsed
			}
		}
	}
	return 0
}

func argBool(args map[string]interface{}, keys ...string) bool {
	for _, k := range keys {
		v, ok := args[k]
		if !ok || v == nil {
			continue
		}
		switch b := v.(type) {
		case bool:
			return b
		case string:
			return strings.EqualFold(strings.TrimSpace(b), "true")
		}
	}
	return false
}

func formatFileRefSummary(fileName string, startLine, endLine int) string {
	if fileName == "" {
		return ""
	}
	if startLine > 0 && endLine > 0 && endLine != startLine {
		return fmt.Sprintf("%s#L%d-%d", fileName, startLine, endLine)
	}
	if startLine > 0 {
		return fmt.Sprintf("%s#L%d", fileName, startLine)
	}
	if endLine > 0 {
		return fmt.Sprintf("%s#L1-%d", fileName, endLine)
	}
	return fileName
}

func extractRunCommandOutput(rc *TrajectoryRunCommand) (string, int) {
	if rc == nil {
		return "", 0
	}
	pick := func(out *TrajectoryCommandOutput) (string, int, bool) {
		if out == nil {
			return "", 0, false
		}
		body := out.Full
		if body == "" {
			body = out.Truncated
		}
		if body == "" && out.NumLinesAbove == 0 {
			return "", 0, false
		}
		return body, out.NumLinesAbove, true
	}
	if body, above, ok := pick(rc.CombinedOutput); ok {
		return truncateTailOutput(body, above, maxToolTerminalOutputLines, maxToolTerminalOutputBytes)
	}
	if body, above, ok := pick(rc.StdoutOutput); ok {
		return truncateTailOutput(body, above, maxToolTerminalOutputLines, maxToolTerminalOutputBytes)
	}
	if body, above, ok := pick(rc.StderrOutput); ok {
		return truncateTailOutput(body, above, maxToolTerminalOutputLines, maxToolTerminalOutputBytes)
	}
	if rc.StdoutBuffer != "" {
		return truncateTailOutput(rc.StdoutBuffer, rc.StdoutLinesAbove, maxToolTerminalOutputLines, maxToolTerminalOutputBytes)
	}
	if rc.StderrBuffer != "" {
		return truncateTailOutput(rc.StderrBuffer, rc.StderrLinesAbove, maxToolTerminalOutputLines, maxToolTerminalOutputBytes)
	}
	return "", 0
}

func resolveToolMetaLabel(s TrajectoryStep, args map[string]interface{}, isRunning bool) string {
	action := strings.TrimSpace(s.Metadata.ToolAction)
	if action == "" && s.ToolCall != nil {
		action = strings.TrimSpace(s.ToolCall.ToolAction)
	}
	if action == "" {
		action = argString(args, "toolAction")
	}
	summary := strings.TrimSpace(s.Metadata.ToolSummary)
	if summary == "" && s.ToolCall != nil {
		summary = strings.TrimSpace(s.ToolCall.ToolSummary)
	}
	if summary == "" {
		summary = argString(args, "toolSummary")
	}
	if isRunning {
		if action != "" {
			return action
		}
		return summary
	}
	if summary != "" {
		return summary
	}
	return action
}

// buildToolDetailItem parses a raw upstream TrajectoryStep into a desktop-parity ToolDetailItem.
// Returns (item, false) for steps that Antigravity Desktop hides (e.g. stepRenderInfo.hidden).
func buildToolDetailItem(s TrajectoryStep, stepIdx int) (ToolDetailItem, bool) {
	if isHiddenToolStep(s) {
		return ToolDetailItem{}, false
	}

	name := extractToolNameFromStep(s)
	lowerName := strings.ToLower(name)
	status := stepDetailStatusLabel(s)
	isRunning := status == "running"
	args := parseToolArgsMap(s)

	item := ToolDetailItem{
		Name:      name,
		Status:    status,
		Duration:  stepDurationLabel(s),
		StepIndex: stepIdx,
		ErrorText: stepErrorShortText(s),
	}

	// 1. Upstream generic StepRenderInfo (Antigravity 2.x native structured render spec)
	var renderInfo *TrajectoryStepRenderInfo
	if s.Generic != nil && s.Generic.Result != nil && s.Generic.Result.StepRenderInfo != nil {
		renderInfo = s.Generic.Result.StepRenderInfo
	}
	if renderInfo != nil {
		applyStepRenderInfo(&item, renderInfo, s, args, isRunning)
		if item.Summary == "" {
			item.Summary = fallbackDetailSummary(item, s)
		}
		return item, true
	}

	// 2. Native typed steps & generic arg fallbacks (never dump raw JSON!)
	switch {
	case s.RunCommand != nil || s.Type == "CORTEX_STEP_TYPE_RUN_COMMAND" ||
		lowerName == "run_command" || lowerName == "shell_command" || lowerName == "command" ||
		s.Type == "CORTEX_STEP_TYPE_COMMAND_STATUS" || lowerName == "command_status":
		fillTerminalDetail(&item, s, args, isRunning)

	case s.ViewFile != nil || s.Type == "CORTEX_STEP_TYPE_VIEW_FILE" ||
		lowerName == "view_file" || lowerName == "read_file":
		fillReadFileDetail(&item, s, args, isRunning)

	case s.CodeAction != nil || s.Type == "CORTEX_STEP_TYPE_CODE_ACTION" ||
		lowerName == "edit_file" || lowerName == "write_file" || lowerName == "write_to_file" ||
		lowerName == "replace_file_content" || lowerName == "multi_replace_file_content" ||
		lowerName == "create_file" || lowerName == "delete_file":
		fillEditFileDetail(&item, s, args, lowerName, isRunning)

	case s.ListDirectory != nil || s.Type == "CORTEX_STEP_TYPE_LIST_DIRECTORY" ||
		lowerName == "list_dir" || lowerName == "list_directory":
		fillListDirDetail(&item, s, args, isRunning)

	case s.GrepSearch != nil || s.CodeSearch != nil || s.Find != nil || s.SearchWeb != nil ||
		s.Type == "CORTEX_STEP_TYPE_GREP_SEARCH" || s.Type == "CORTEX_STEP_TYPE_CODE_SEARCH" ||
		s.Type == "CORTEX_STEP_TYPE_FIND" || s.Type == "CORTEX_STEP_TYPE_SEARCH_WEB" ||
		lowerName == "grep_search" || lowerName == "search_code" || lowerName == "find" ||
		lowerName == "find_by_name" || lowerName == "file_search" || lowerName == "search_web":
		fillSearchDetail(&item, s, args, lowerName, isRunning)

	case s.ReadURLContent != nil || s.Type == "CORTEX_STEP_TYPE_READ_URL_CONTENT" || lowerName == "read_url_content":
		item.Kind = "read"
		if isRunning {
			item.Prefix = "Reading"
		} else {
			item.Prefix = "Read"
		}
		u := ""
		if s.ReadURLContent != nil {
			u = strings.TrimSpace(s.ReadURLContent.URL)
		}
		if u == "" {
			u = argString(args, "Url", "url", "URL")
		}
		item.Title = u
		item.Summary = u

	case s.McpTool != nil || s.Type == "CORTEX_STEP_TYPE_MCP_TOOL" ||
		lowerName == "call_mcp_tool" || lowerName == "mcp_tool" ||
		lowerName == "list_resources" || lowerName == "read_resource":
		fillMcpDetail(&item, s, args, lowerName)

	default:
		item.Kind = "other"
		label := resolveToolMetaLabel(s, args, isRunning)
		if label == "" && s.TaskBoundary != nil && strings.TrimSpace(s.TaskBoundary.TaskName) != "" {
			label = strings.TrimSpace(s.TaskBoundary.TaskName)
		}
		if label != "" {
			item.Title = label
			item.Summary = label
		}
	}

	if item.Summary == "" {
		item.Summary = fallbackDetailSummary(item, s)
	}
	return item, true
}

func applyStepRenderInfo(item *ToolDetailItem, ri *TrajectoryStepRenderInfo, s TrajectoryStep, args map[string]interface{}, isRunning bool) {
	lowerName := strings.ToLower(item.Name)

	// Prefix alignment mirroring desktop X7c
	prefix := strings.TrimSpace(ri.TitlePrefix)
	switch item.Status {
	case "rejected":
		prefix = "Rejected"
	case "canceled":
		prefix = "Canceled"
	case "interrupted":
		prefix = "Interrupted"
	case "done":
		if strings.EqualFold(prefix, "Running") {
			prefix = "Ran"
		}
	}
	item.Prefix = prefix

	// Parse titleParts (text, file, fileDiff, badge)
	var textParts []string
	hasFileDiff := false
	hasFileRef := false
	for _, tp := range ri.TitleParts {
		if t := strings.TrimSpace(tp.Text); t != "" {
			textParts = append(textParts, t)
		}
		if tp.File != nil && strings.TrimSpace(tp.File.URI) != "" {
			hasFileRef = true
			if item.FileURI == "" {
				item.FileURI = normalizeFileURI(tp.File.URI)
				item.FileName = basenameOfFileURI(tp.File.URI)
				item.StartLine = tp.File.StartLine
				item.EndLine = tp.File.EndLine
				item.IsArtifact = isBrainArtifactURI(item.FileURI)
			} else {
				textParts = append(textParts, formatFileRefSummary(basenameOfFileURI(tp.File.URI), tp.File.StartLine, tp.File.EndLine))
			}
		}
		if tp.FileDiff != nil && strings.TrimSpace(tp.FileDiff.URI) != "" {
			hasFileDiff = true
			if item.FileURI == "" {
				item.FileURI = normalizeFileURI(tp.FileDiff.URI)
				item.FileName = basenameOfFileURI(tp.FileDiff.URI)
				item.IsArtifact = isBrainArtifactURI(item.FileURI)
			}
			if tp.FileDiff.DiffStats != nil {
				item.Additions = tp.FileDiff.DiffStats.Additions
				item.Deletions = tp.FileDiff.DiffStats.Deletions
				item.HasDiffStats = true
			}
		}
		if tp.Badge != nil && strings.TrimSpace(tp.Badge.Label) != "" {
			textParts = append(textParts, strings.TrimSpace(tp.Badge.Label))
		}
	}

	if len(textParts) > 0 {
		item.Title = strings.Join(textParts, " ")
	} else if t := strings.TrimSpace(ri.Title); t != "" {
		item.Title = t
	}

	// Parse contentBlocks (terminal, markdown)
	hasTerminalBlock := false
	var mdBlocks []string
	for _, cb := range ri.ContentBlocks {
		if cb.Terminal != nil {
			hasTerminalBlock = true
			if item.Command == "" {
				item.Command = strings.TrimSpace(cb.Terminal.Command)
			}
			if item.Cwd == "" {
				item.Cwd = shortenCwd(cb.Terminal.Cwd)
			}
			if cb.Terminal.ExitCode != nil {
				ec := *cb.Terminal.ExitCode
				item.ExitCode = &ec
			}
			if cb.Terminal.Output != "" || cb.Terminal.NumLinesAbove > 0 {
				out, above := truncateTailOutput(cb.Terminal.Output, cb.Terminal.NumLinesAbove, maxToolTerminalOutputLines, maxToolTerminalOutputBytes)
				item.Output = out
				item.NumLinesAbove = above
			}
		}
		if md := strings.TrimSpace(cb.Markdown); md != "" {
			mdBlocks = append(mdBlocks, md)
		}
	}
	if len(mdBlocks) > 0 {
		item.Markdown = truncateMarkdown(strings.Join(mdBlocks, "\n\n"), maxToolMarkdownBytes)
	} else if md := strings.TrimSpace(ri.Markdown); md != "" {
		item.Markdown = truncateMarkdown(md, maxToolMarkdownBytes)
	}

	// Classify Kind matching desktop mY / y5b
	agg := strings.ToLower(strings.TrimSpace(ri.AggregationKey))
	switch {
	case hasFileDiff || agg == "edits" || lowerName == "edit_file" || lowerName == "write_file" || lowerName == "write_to_file" || lowerName == "replace_file_content" || lowerName == "multi_replace_file_content" || lowerName == "delete_file":
		item.Kind = "edit"
	case hasTerminalBlock || agg == "terminal" || lowerName == "run_command" || lowerName == "shell_command":
		item.Kind = "terminal"
		if item.Command == "" {
			if s.RunCommand != nil {
				item.Command = strings.TrimSpace(s.RunCommand.CommandLine)
			}
			if item.Command == "" {
				item.Command = argString(args, "CommandLine", "commandLine", "command")
			}
		}
		if item.Cwd == "" {
			if s.RunCommand != nil {
				item.Cwd = shortenCwd(s.RunCommand.Cwd)
			}
			if item.Cwd == "" {
				item.Cwd = shortenCwd(argString(args, "Cwd", "cwd"))
			}
		}
		if isGenericCommandNoise(item.Title) {
			meta := resolveToolMetaLabel(s, args, isRunning)
			if !isGenericCommandNoise(meta) {
				item.Title = meta
			} else {
				item.Title = ""
			}
		}
	case agg == "folders" || lowerName == "list_dir" || lowerName == "list_directory":
		item.Kind = "list"
	case hasFileRef || agg == "files" || lowerName == "read_file" || lowerName == "view_file":
		item.Kind = "read"
	case strings.Contains(lowerName, "search") || strings.Contains(lowerName, "grep") || strings.Contains(lowerName, "find"):
		item.Kind = "search"
	case strings.HasPrefix(lowerName, "mcp_") || lowerName == "call_mcp_tool" || lowerName == "mcp_tool":
		item.Kind = "mcp"
	default:
		item.Kind = "other"
	}

	if item.Title == "" && item.FileName == "" && item.Command == "" {
		if meta := resolveToolMetaLabel(s, args, isRunning); meta != "" {
			item.Title = meta
		}
	}
}

func fillTerminalDetail(item *ToolDetailItem, s TrajectoryStep, args map[string]interface{}, isRunning bool) {
	item.Kind = "terminal"
	cmd := ""
	cwd := ""
	if s.RunCommand != nil {
		cmd = strings.TrimSpace(s.RunCommand.CommandLine)
		if cmd == "" {
			cmd = strings.TrimSpace(s.RunCommand.ProposedCommandLine)
		}
		cwd = shortenCwd(s.RunCommand.Cwd)
		if s.RunCommand.ExitCode != nil {
			ec := *s.RunCommand.ExitCode
			item.ExitCode = &ec
		}
		item.Output, item.NumLinesAbove = extractRunCommandOutput(s.RunCommand)
	}
	if cmd == "" {
		cmd = argString(args, "CommandLine", "commandLine", "command")
	}
	if cmd == "" && s.TaskDetails != nil {
		cmd = strings.TrimSpace(s.TaskDetails.Description)
	}
	if cwd == "" {
		cwd = shortenCwd(argString(args, "Cwd", "cwd"))
	}
	item.Command = cmd
	item.Cwd = cwd

	meta := resolveToolMetaLabel(s, args, isRunning)
	if !isGenericCommandNoise(meta) {
		item.Title = meta
	}

	switch item.Status {
	case "rejected":
		item.Prefix = "Rejected"
	case "canceled":
		item.Prefix = "Canceled"
	case "interrupted":
		item.Prefix = "Interrupted"
	case "error":
		item.Prefix = "Errored"
	case "waiting":
		item.Prefix = "Run"
	case "running":
		item.Prefix = "Running"
	default:
		item.Prefix = "Ran"
	}
	// If Title already starts with the same verb (e.g. "Ran command"), avoid duplicating prefix.
	if item.Title != "" && strings.HasPrefix(strings.ToLower(item.Title), strings.ToLower(item.Prefix)+" ") {
		item.Prefix = ""
	}

	if meta != "" {
		item.Summary = meta
	} else if cmd != "" {
		item.Summary = cmd
	}
}

func fillReadFileDetail(item *ToolDetailItem, s TrajectoryStep, args map[string]interface{}, isRunning bool) {
	item.Kind = "read"
	if isRunning {
		item.Prefix = "Analyzing"
	} else {
		item.Prefix = "Analyzed"
	}
	rawPath := ""
	startLine := 0
	endLine := 0
	if s.ViewFile != nil {
		rawPath = s.ViewFile.AbsolutePathURI
		startLine = s.ViewFile.StartLine
		endLine = s.ViewFile.EndLine
	}
	if rawPath == "" {
		rawPath = argString(args, "Path", "AbsolutePath", "path", "absolutePathUri", "TargetFile")
	}
	if startLine == 0 {
		startLine = argInt(args, "StartLine", "start_line", "startLine")
	}
	if endLine == 0 {
		endLine = argInt(args, "EndLine", "end_line", "endLine")
	}
	if rawPath != "" {
		item.FileURI = normalizeFileURI(rawPath)
		item.FileName = basenameOfFileURI(rawPath)
		item.StartLine = startLine
		item.EndLine = endLine
		item.IsArtifact = isBrainArtifactURI(item.FileURI)
		item.Summary = formatFileRefSummary(item.FileName, startLine, endLine)
	} else if meta := resolveToolMetaLabel(s, args, isRunning); meta != "" {
		item.Title = meta
		item.Summary = meta
	}
}

func fillEditFileDetail(item *ToolDetailItem, s TrajectoryStep, args map[string]interface{}, lowerName string, isRunning bool) {
	item.Kind = "edit"
	isCreate := false
	isDelete := false
	rawPath := ""

	if s.CodeAction != nil {
		ca := s.CodeAction
		item.IsArtifact = ca.IsArtifactFile
		if ca.DiffStats != nil {
			item.Additions = ca.DiffStats.Additions
			item.Deletions = ca.DiffStats.Deletions
			item.HasDiffStats = true
		}
		if ca.ActionResult != nil {
			if ca.ActionResult.Edit != nil && ca.ActionResult.Edit.AbsoluteURI != "" {
				rawPath = ca.ActionResult.Edit.AbsoluteURI
				if ca.ActionResult.Edit.CreateFile {
					isCreate = true
				}
			} else if ca.ActionResult.AbsoluteURI != "" {
				rawPath = ca.ActionResult.AbsoluteURI
			}
		}
		if ca.ActionSpec != nil {
			if ca.ActionSpec.DeleteFile != nil {
				isDelete = true
				if rawPath == "" && ca.ActionSpec.DeleteFile.Path != nil {
					rawPath = ca.ActionSpec.DeleteFile.Path.AbsoluteURI
				}
			} else if ca.ActionSpec.CreateFile != nil {
				if !ca.ActionSpec.CreateFile.Overwrite {
					isCreate = true
				}
				if rawPath == "" && ca.ActionSpec.CreateFile.Path != nil {
					rawPath = ca.ActionSpec.CreateFile.Path.AbsoluteURI
				}
			} else if ca.ActionSpec.EditFile != nil && rawPath == "" && ca.ActionSpec.EditFile.Path != nil {
				rawPath = ca.ActionSpec.EditFile.Path.AbsoluteURI
			}
		}
	}

	if rawPath == "" {
		rawPath = argString(args, "Path", "TargetFile", "targetFile", "AbsolutePath", "path")
	}
	if lowerName == "delete_file" {
		isDelete = true
	} else if (lowerName == "write_file" || lowerName == "write_to_file" || lowerName == "create_file") && !argBool(args, "Overwrite", "overwrite") {
		isCreate = true
	}

	switch {
	case item.Status == "canceled":
		if isDelete {
			item.Prefix = "Canceled deletion of"
		} else if isCreate {
			item.Prefix = "Canceled creation of"
		} else {
			item.Prefix = "Canceled edit to"
		}
	case isDelete:
		if isRunning {
			item.Prefix = "Deleting"
		} else {
			item.Prefix = "Deleted"
		}
	case isCreate:
		if isRunning {
			item.Prefix = "Creating"
		} else {
			item.Prefix = "Created"
		}
	default:
		if isRunning {
			item.Prefix = "Editing"
		} else {
			item.Prefix = "Edited"
		}
	}

	if rawPath != "" {
		item.FileURI = normalizeFileURI(rawPath)
		item.FileName = basenameOfFileURI(rawPath)
		if !item.IsArtifact {
			item.IsArtifact = isBrainArtifactURI(item.FileURI)
		}
		item.Summary = item.FileName
	} else if meta := resolveToolMetaLabel(s, args, isRunning); meta != "" {
		item.Title = meta
		item.Summary = meta
	}
}

func fillListDirDetail(item *ToolDetailItem, s TrajectoryStep, args map[string]interface{}, isRunning bool) {
	item.Kind = "list"
	if isRunning {
		item.Prefix = "Analyzing"
	} else {
		item.Prefix = "Analyzed"
	}
	rawPath := ""
	if s.ListDirectory != nil {
		rawPath = s.ListDirectory.DirectoryPathURI
		item.NumResults = len(s.ListDirectory.Results)
	}
	if rawPath == "" {
		rawPath = argString(args, "DirectoryPath", "Path", "directoryPathUri", "path")
	}
	if rawPath != "" {
		item.FileURI = normalizeFileURI(rawPath)
		item.FileName = basenameOfFileURI(rawPath)
		item.Summary = item.FileName
	} else if meta := resolveToolMetaLabel(s, args, isRunning); meta != "" {
		item.Title = meta
		item.Summary = meta
	}
}

func fillSearchDetail(item *ToolDetailItem, s TrajectoryStep, args map[string]interface{}, lowerName string, isRunning bool) {
	item.Kind = "search"
	if isRunning {
		item.Prefix = "Searching"
	} else {
		item.Prefix = "Searched"
	}
	query := ""
	switch {
	case s.GrepSearch != nil:
		query = strings.TrimSpace(s.GrepSearch.Query)
		item.NumResults = s.GrepSearch.TotalResults
	case s.CodeSearch != nil:
		query = strings.TrimSpace(s.CodeSearch.Query)
	case s.Find != nil:
		query = strings.TrimSpace(s.Find.Pattern)
		item.NumResults = s.Find.TotalResults
	case s.SearchWeb != nil:
		q := strings.TrimSpace(s.SearchWeb.Query)
		d := strings.TrimSpace(s.SearchWeb.Domain)
		if d != "" && q != "" {
			query = fmt.Sprintf(`%s for "%s"`, d, q)
		} else if q != "" {
			query = fmt.Sprintf(`web for "%s"`, q)
		}
	}
	if query == "" {
		query = argString(args, "Query", "query", "Pattern", "pattern")
		if query != "" && (lowerName == "search_web") {
			if d := argString(args, "domain", "Domain"); d != "" {
				query = fmt.Sprintf(`%s for "%s"`, d, query)
			} else {
				query = fmt.Sprintf(`web for "%s"`, query)
			}
		}
	}
	if query != "" {
		item.Title = query
		item.Summary = query
	} else if meta := resolveToolMetaLabel(s, args, isRunning); meta != "" {
		item.Title = meta
		item.Summary = meta
	}
}

func fillMcpDetail(item *ToolDetailItem, s TrajectoryStep, args map[string]interface{}, lowerName string) {
	item.Kind = "mcp"
	server := ""
	tool := ""
	if s.McpTool != nil {
		server = strings.TrimSpace(s.McpTool.ServerName)
		tool = strings.TrimSpace(s.McpTool.ToolName)
	}
	if server == "" {
		server = argString(args, "ServerName", "serverName")
	}
	if tool == "" {
		tool = argString(args, "ToolName", "toolName")
	}
	if server != "" {
		item.Prefix = server
	} else {
		item.Prefix = "MCP"
	}
	if tool != "" {
		item.Title = tool
		item.Summary = tool
	} else if meta := resolveToolMetaLabel(s, args, item.Status == "running"); meta != "" {
		item.Title = meta
		item.Summary = meta
	} else if lowerName != "" && lowerName != "call_mcp_tool" && lowerName != "mcp_tool" {
		item.Title = strings.TrimPrefix(lowerName, "mcp_")
		item.Summary = item.Title
	}
}

func fallbackDetailSummary(item ToolDetailItem, s TrajectoryStep) string {
	if item.FileName != "" {
		return formatFileRefSummary(item.FileName, item.StartLine, item.EndLine)
	}
	if item.Title != "" {
		return item.Title
	}
	if item.Command != "" {
		return item.Command
	}
	if s.Metadata.ToolSummary != "" {
		return strings.TrimSpace(s.Metadata.ToolSummary)
	}
	if s.Metadata.ToolAction != "" {
		return strings.TrimSpace(s.Metadata.ToolAction)
	}
	if s.TaskBoundary != nil && s.TaskBoundary.TaskName != "" {
		return strings.TrimSpace(s.TaskBoundary.TaskName)
	}
	return ""
}

func hashToolDetailItem(h *fnv64, td *ToolDetailItem) {
	h.str(td.Name)
	h.str(td.Summary)
	h.str(td.Status)
	h.str(td.Duration)
	h.num(td.StepIndex)
	h.str(td.Kind)
	h.str(td.Prefix)
	h.str(td.Title)
	h.str(td.FileURI)
	h.str(td.FileName)
	h.num(td.StartLine)
	h.num(td.EndLine)
	h.num(td.Additions)
	h.num(td.Deletions)
	h.flag(td.HasDiffStats)
	h.num(td.NumResults)
	h.str(td.Command)
	h.str(td.Cwd)
	h.str(td.Output)
	h.num(td.NumLinesAbove)
	if td.ExitCode != nil {
		h.u8(1)
		h.num(*td.ExitCode)
	} else {
		h.u8(0)
	}
	h.str(td.Markdown)
	h.str(td.ErrorText)
	h.flag(td.IsArtifact)
}

func hashThinkingBlockItem(h *fnv64, tb *ThinkingBlockItem) {
	h.num(tb.StepIndex)
	h.str(tb.Text)
	h.str(tb.Duration)
	h.str(tb.Status)
}
