package proxy

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"antigravity-mobile/internal/i18n"
)

const (
	postCompactSystemBaselineTokens int64 = 6000
	topConsumerMinTokens            int64 = 300
	topConsumerMaxItems                   = 3
)

// estimateTextTokens provides a fast CJK-aware token estimate (~1.2 tok/CJK rune, ~0.25 tok/ASCII char).
func estimateTextTokens(s string) int64 {
	if s == "" {
		return 0
	}
	var cjk, other int64
	for _, r := range s {
		if (r >= 0x2e80 && r <= 0x9fff) || (r >= 0xf900 && r <= 0xfaff) || (r >= 0x3040 && r <= 0x30ff) || (r >= 0xac00 && r <= 0xd7af) {
			cjk++
		} else {
			other++
		}
	}
	est := int64(math.Round(float64(other)/4.0 + float64(cjk)*1.2))
	if est < 1 {
		est = 1
	}
	return est
}

// estimateAttachmentFileTokens returns a conservative token estimate for an uploaded attachment.
func estimateAttachmentFileTokens(filePath, name, mimeType, kind string, size int64) int64 {
	if size <= 0 {
		return 0
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	mimeLower := strings.ToLower(strings.TrimSpace(mimeType))

	// 1. Images: decode header dimensions when possible (standard ~w*h/750 tile approximation).
	if strings.HasPrefix(mimeLower, "image/") || ext == "png" || ext == "jpg" || ext == "jpeg" || ext == "gif" || ext == "webp" || ext == "heic" {
		if filePath != "" {
			if f, err := os.Open(filePath); err == nil {
				cfg, _, decErr := image.DecodeConfig(f)
				_ = f.Close()
				if decErr == nil && cfg.Width > 0 && cfg.Height > 0 {
					w, h := cfg.Width, cfg.Height
					// Cap long edge at 2048px as models downscale ultra-high-res images.
					if w > 2048 || h > 2048 {
						scale := 2048.0 / float64(max(w, h))
						w = int(math.Round(float64(w) * scale))
						h = int(math.Round(float64(h) * scale))
					}
					tok := int64(math.Round(float64(w*h) / 750.0))
					if tok < 258 {
						tok = 258
					}
					if tok > 4096 {
						tok = 4096
					}
					return tok
				}
			}
		}
		tok := size / 1500
		if tok < 258 {
			tok = 258
		}
		if tok > 2048 {
			tok = 2048
		}
		return tok
	}

	// 2. Plain text / code / markdown / CSV / JSON: sample up to 256 KB for accurate CJK density.
	isTextLike := kind == "code" || strings.HasPrefix(mimeLower, "text/") ||
		ext == "txt" || ext == "md" || ext == "markdown" || ext == "log" || ext == "csv" || ext == "tsv" || ext == "json" || ext == "jsonl" || ext == "xml" || ext == "yaml" || ext == "yml"
	if isTextLike {
		if filePath != "" {
			if f, err := os.Open(filePath); err == nil {
				sample, _ := io.ReadAll(io.LimitReader(f, 256*1024))
				_ = f.Close()
				if len(sample) > 0 {
					sampleTok := estimateTextTokens(string(sample))
					if int64(len(sample)) >= size {
						return sampleTok
					}
					return int64(math.Round(float64(sampleTok) * (float64(size) / float64(len(sample)))))
				}
			}
		}
		return max(1, size/4)
	}

	// 3. PDF & Office documents (compressed containers: rough text-density approximation).
	switch ext {
	case "pdf":
		// Typical text-bearing PDF averages ~1 token per 24 bytes (bounded cleanly for scanned PDFs).
		return max(250, min(size/24, 500000))
	case "docx", "doc", "pptx", "ppt", "xlsx", "xls", "xlsm", "odt", "rtf", "epub":
		return max(150, min(size/10, 500000))
	case "mp3", "m4a", "wav", "aac", "ogg", "amr", "flac":
		// Audio ~32 tokens/sec, ~16 KB/sec compressed => ~1 token per 512 bytes.
		return max(64, min(size/512, 200000))
	case "mp4", "mov", "webm", "mkv":
		// Video ~263 tokens/sec, ~300 KB/sec => ~1 token per 1200 bytes.
		return max(256, min(size/1200, 300000))
	default:
		return 0
	}
}

func isSuccessfulCheckpointStep(s TrajectoryStep) bool {
	if s.Type != "CORTEX_STEP_TYPE_CHECKPOINT" && s.Checkpoint == nil {
		return false
	}
	st := strings.ToUpper(strings.TrimSpace(s.Status))
	for _, bad := range []string{"RUNNING", "PENDING", "GENERATING", "IN_PROGRESS", "STREAMING", "QUEUED", "ERROR", "FAILED", "CANCELED", "CANCELLED", "ABORTED"} {
		if strings.HasSuffix(st, bad) {
			return false
		}
	}
	if s.Checkpoint != nil && s.Checkpoint.ModelSummarizationFailed {
		return false
	}
	return true
}

func estimateCheckpointSummaryTokens(s TrajectoryStep) int64 {
	var total int64
	if s.Checkpoint != nil {
		total += estimateTextTokens(s.Checkpoint.SessionSummary)
		total += estimateTextTokens(s.Checkpoint.CodeChangeSummary)
		total += estimateTextTokens(s.Checkpoint.UserIntent)
		total += estimateTextTokens(s.Checkpoint.MemorySummary)
	}
	if s.Content != "" {
		total += estimateTextTokens(s.Content)
	}
	if s.SystemMessage != nil {
		total += estimateTextTokens(s.SystemMessage.Content)
		total += estimateTextTokens(s.SystemMessage.Message)
	}
	return total
}

func estimateSingleStepContribution(stepIdx int, s TrajectoryStep, lang string) (category string, kind string, label string, tokens int64) {
	if s.UserInput != nil {
		text, _ := s.UserInput.ExtractTextAndSlashCommands()
		tok := estimateTextTokens(text)
		mediaTok := int64(len(s.UserInput.Images)+len(s.UserInput.Media)) * 1024
		if mediaTok > tok {
			return "system_attachments", "attachment", summarizeConsumerLabel(i18n.T3For(lang, "附件输入", "Attachment", "添付ファイル"), text), tok + mediaTok
		}
		return "conversation", "user", summarizeConsumerLabel(i18n.T3For(lang, "用户消息", "User Message", "ユーザーメッセージ"), text), tok + mediaTok
	}

	if s.PlannerResponse != nil {
		thinkText := s.PlannerResponse.Thinking
		if thinkText == "" {
			thinkText = s.PlannerResponse.RawThinking
		}
		thinkTok := estimateTextTokens(thinkText)
		respTok := estimateTextTokens(s.PlannerResponse.Response)
		if thinkTok > respTok {
			return "thinking", "thinking", summarizeConsumerLabel(i18n.T3For(lang, "深度思考", "Thinking", "思考プロセス"), thinkText), thinkTok
		}
		return "conversation", "agent", summarizeConsumerLabel(i18n.T3For(lang, "助手回复", "Assistant Reply", "アシスタント応答"), s.PlannerResponse.Response), respTok
	}

	if s.RunCommand != nil {
		var tok int64
		tok += estimateTextTokens(s.RunCommand.CommandLine)
		if s.RunCommand.CombinedOutput != nil {
			tok += estimateTextTokens(s.RunCommand.CombinedOutput.Full)
			if s.RunCommand.CombinedOutput.Full == "" {
				tok += estimateTextTokens(s.RunCommand.CombinedOutput.Truncated)
			}
		}
		if s.RunCommand.StdoutOutput != nil {
			tok += estimateTextTokens(s.RunCommand.StdoutOutput.Full)
			if s.RunCommand.StdoutOutput.Full == "" {
				tok += estimateTextTokens(s.RunCommand.StdoutOutput.Truncated)
			}
		}
		if s.RunCommand.StderrOutput != nil {
			tok += estimateTextTokens(s.RunCommand.StderrOutput.Full)
			if s.RunCommand.StderrOutput.Full == "" {
				tok += estimateTextTokens(s.RunCommand.StderrOutput.Truncated)
			}
		}
		tok += estimateTextTokens(s.RunCommand.StdoutBuffer) + estimateTextTokens(s.RunCommand.StderrBuffer)
		if s.Generic != nil && s.Generic.Result != nil && s.Generic.Result.StepRenderInfo != nil {
			tok += estimateTextTokens(s.Generic.Result.StepRenderInfo.Markdown)
		}
		cmd := strings.TrimSpace(s.RunCommand.CommandLine)
		if cmd == "" {
			cmd = strings.TrimSpace(s.Metadata.ToolSummary)
		}
		return "tools", "tool", summarizeConsumerLabel(i18n.T3For(lang, "终端命令", "Terminal", "ターミナル"), cmd), tok
	}

	if s.CaptureBrowserScreenshot != nil {
		return "system_attachments", "attachment", summarizeConsumerLabel(i18n.T3For(lang, "网页截图", "Web Screenshot", "スクリーンショット"), s.CaptureBrowserScreenshot.PageTitle), 1200
	}

	if s.SystemMessage != nil || s.Checkpoint != nil || s.Type == "CORTEX_STEP_TYPE_CHECKPOINT" {
		tok := estimateCheckpointSummaryTokens(s)
		return "system_attachments", "attachment", summarizeConsumerLabel(i18n.T3For(lang, "系统上下文", "System Context", "システムコンテキスト"), s.Content), tok
	}

	// Generic tool calls (view_file, grep_search, code_action, mcp, etc.)
	toolName := extractToolNameFromStep(s)
	var tok int64
	if args := extractStepToolArgsJSON(s); args != "" {
		tok += estimateTextTokens(args)
	}
	if s.Content != "" {
		tok += estimateTextTokens(s.Content)
	}
	if s.Generic != nil && s.Generic.Result != nil && s.Generic.Result.StepRenderInfo != nil {
		ri := s.Generic.Result.StepRenderInfo
		tok += estimateTextTokens(ri.Markdown)
		for _, cb := range ri.ContentBlocks {
			tok += estimateTextTokens(cb.Markdown)
			if cb.Terminal != nil {
				tok += estimateTextTokens(cb.Terminal.Output)
			}
		}
	}
	// Estimate read_file / view_file span when raw file content is omitted from trajectory JSON.
	if s.ViewFile != nil && s.ViewFile.EndLine >= s.ViewFile.StartLine && s.ViewFile.EndLine > 0 {
		lines := s.ViewFile.EndLine - s.ViewFile.StartLine + 1
		if lines > 0 {
			spanTok := int64(lines) * 14
			if spanTok > tok {
				tok = spanTok
			}
		}
	}
	if s.CodeAction != nil && s.CodeAction.DiffStats != nil {
		diffLines := s.CodeAction.DiffStats.Additions + s.CodeAction.DiffStats.Deletions
		if diffTok := int64(diffLines) * 16; diffTok > tok {
			tok = diffTok
		}
	}

	target := strings.TrimSpace(s.Metadata.ToolSummary)
	if target == "" && s.ViewFile != nil {
		target = filepath.Base(uriToFilePath(s.ViewFile.AbsolutePathURI))
	}
	if target == "" && s.GrepSearch != nil {
		target = s.GrepSearch.Query
	}
	if target == "" && s.McpTool != nil {
		target = s.McpTool.ToolName
	}
	if target == "" && s.CodeAction != nil && s.CodeAction.ActionResult != nil {
		if s.CodeAction.ActionResult.Edit != nil && s.CodeAction.ActionResult.Edit.AbsoluteURI != "" {
			target = filepath.Base(uriToFilePath(s.CodeAction.ActionResult.Edit.AbsoluteURI))
		} else if s.CodeAction.ActionResult.AbsoluteURI != "" {
			target = filepath.Base(uriToFilePath(s.CodeAction.ActionResult.AbsoluteURI))
		}
	}
	prefix := toolName
	if prefix == "" {
		prefix = "tool"
	}
	return "tools", "tool", summarizeConsumerLabel(prefix, target), tok
}

func summarizeConsumerLabel(prefix, detail string) string {
	d := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(detail, "\r", " "), "\n", " "))
	for strings.Contains(d, "  ") {
		d = strings.ReplaceAll(d, "  ", " ")
	}
	if d == "" {
		return prefix
	}
	runes := []rune(d)
	if len(runes) > 36 {
		d = string(runes[:35]) + "…"
	}
	if prefix == "" || strings.EqualFold(prefix, d) {
		return d
	}
	return prefix + " · " + d
}

// resolveStepMessageID finds the CascadeMessageItem.ID that contains or immediately follows stepIndex.
func resolveStepMessageID(stepIdx int, messages []CascadeMessageItem) string {
	if len(messages) == 0 {
		return ""
	}
	for _, m := range messages {
		if m.StepIndex != nil && *m.StepIndex == stepIdx {
			return m.ID
		}
		for _, td := range m.ToolDetails {
			if td.StepIndex == stepIdx {
				return m.ID
			}
		}
		for _, tb := range m.ThinkingBlocks {
			if tb.StepIndex == stepIdx {
				return m.ID
			}
		}
	}
	// Nearest message at or after stepIdx, otherwise last message before it.
	bestAfterID := ""
	bestAfterStep := int(^uint(0) >> 1)
	bestBeforeID := ""
	bestBeforeStep := -1
	for _, m := range messages {
		if m.StepIndex == nil {
			continue
		}
		si := *m.StepIndex
		if si >= stepIdx && si < bestAfterStep {
			bestAfterStep = si
			bestAfterID = m.ID
		}
		if si <= stepIdx && si > bestBeforeStep {
			bestBeforeStep = si
			bestBeforeID = m.ID
		}
	}
	if bestBeforeID != "" {
		return bestBeforeID
	}
	return bestAfterID
}

// applyTrajectoryContextEnrichment performs two mobile-focused enhancements on ContextMeterItem
// without mutating the cached base object:
//  1. Immediate post-compact watermark drop when a successful CHECKPOINT occurred after the last
//     model response (so the context ring drops right away after `/compact`).
//  2. Lightweight 4-category context composition breakdown + Top-3 token-heavy steps.
func applyTrajectoryContextEnrichment(base *ContextMeterItem, steps []TrajectoryStep, messages []CascadeMessageItem) *ContextMeterItem {
	return applyTrajectoryContextEnrichmentForLang(base, steps, messages, i18n.GetLang())
}

func applyTrajectoryContextEnrichmentForLang(base *ContextMeterItem, steps []TrajectoryStep, messages []CascadeMessageItem, lang string) *ContextMeterItem {
	resolvedLang := i18n.ResolveLang(lang)
	if base == nil && len(steps) == 0 {
		return nil
	}
	if len(steps) == 0 {
		return base
	}

	cpCount := 0
	lastCPIdx := -1
	hasPlannerAfterCP := false
	for i, s := range steps {
		if isSuccessfulCheckpointStep(s) {
			cpCount++
			lastCPIdx = i
			hasPlannerAfterCP = false
			continue
		}
		if lastCPIdx >= 0 && s.PlannerResponse != nil &&
			(strings.TrimSpace(s.PlannerResponse.Response) != "" || strings.TrimSpace(s.PlannerResponse.Thinking) != "") {
			hasPlannerAfterCP = true
		}
	}

	startIdx := 0
	if lastCPIdx >= 0 {
		startIdx = lastCPIdx
	}

	catTotals := map[string]int64{
		"tools":              0,
		"conversation":       0,
		"thinking":           0,
		"system_attachments": 0,
	}
	var consumers []ContextTopConsumer
	var estimatedActiveTotal int64

	for i := startIdx; i < len(steps); i++ {
		s := steps[i]
		// For PlannerResponse steps that have both thinking and response, attribute both categories accurately.
		if s.PlannerResponse != nil {
			thinkText := s.PlannerResponse.Thinking
			if thinkText == "" {
				thinkText = s.PlannerResponse.RawThinking
			}
			thinkTok := estimateTextTokens(thinkText)
			respTok := estimateTextTokens(s.PlannerResponse.Response)
			if thinkTok > 0 {
				catTotals["thinking"] += thinkTok
				estimatedActiveTotal += thinkTok
				if thinkTok >= topConsumerMinTokens {
					consumers = append(consumers, ContextTopConsumer{
						StepIndex: i,
						MessageID: resolveStepMessageID(i, messages),
						Kind:      "thinking",
						Label:     summarizeConsumerLabel(i18n.T3For(resolvedLang, "深度思考", "Thinking", "思考プロセス"), thinkText),
						Tokens:    thinkTok,
					})
				}
			}
			if respTok > 0 {
				catTotals["conversation"] += respTok
				estimatedActiveTotal += respTok
				if respTok >= topConsumerMinTokens {
					consumers = append(consumers, ContextTopConsumer{
						StepIndex: i,
						MessageID: resolveStepMessageID(i, messages),
						Kind:      "agent",
						Label:     summarizeConsumerLabel(i18n.T3For(resolvedLang, "助手回复", "Assistant Reply", "アシスタント応答"), s.PlannerResponse.Response),
						Tokens:    respTok,
					})
				}
			}
			continue
		}

		cat, kind, label, tok := estimateSingleStepContribution(i, s, resolvedLang)
		if tok <= 0 {
			continue
		}
		catTotals[cat] += tok
		estimatedActiveTotal += tok
		// Exclude synthetic checkpoint summary from Top Consumers list so users see actionable steps.
		if i != lastCPIdx && tok >= topConsumerMinTokens {
			consumers = append(consumers, ContextTopConsumer{
				StepIndex: i,
				MessageID: resolveStepMessageID(i, messages),
				Kind:      kind,
				Label:     label,
				Tokens:    tok,
			})
		}
	}

	var out ContextMeterItem
	if base != nil {
		out = *base
	} else {
		out.MaxTokens = 200000
	}

	if cpCount > 0 {
		out.CheckpointCount = cpCount
		idxCopy := lastCPIdx
		out.LastCheckpointStep = &idxCopy
	}

	// 1. Immediate Post-Compact watermark drop when no new model call has completed after the checkpoint.
	if lastCPIdx >= 0 && !hasPlannerAfterCP {
		postCompactEst := estimatedActiveTotal + postCompactSystemBaselineTokens
		if out.MaxTokens <= 0 {
			out.MaxTokens = 200000
		}
		if postCompactEst > 0 && (out.UsedTokens == 0 || postCompactEst < out.UsedTokens) {
			out.UsedTokens = postCompactEst
			pct := float64(postCompactEst) / float64(out.MaxTokens) * 100.0
			if pct > 100.0 {
				pct = 100.0
			}
			out.Percentage = math.Round(pct*10) / 10
			out.IsOverflow = false
			out.Compacted = true
		}
	}

	// 2. Reconcile 4-category breakdown against UsedTokens so bars sum cleanly to 100%.
	denom := out.UsedTokens
	if denom <= 0 {
		denom = estimatedActiveTotal
	}
	if denom > 0 {
		if estimatedActiveTotal < denom {
			catTotals["system_attachments"] += denom - estimatedActiveTotal
		} else if estimatedActiveTotal > denom && estimatedActiveTotal > 0 {
			// Scale estimates proportionally down to actual server-reported UsedTokens.
			scale := float64(denom) / float64(estimatedActiveTotal)
			for k, v := range catTotals {
				catTotals[k] = int64(math.Round(float64(v) * scale))
			}
		}

		order := []string{"tools", "conversation", "thinking", "system_attachments"}
		cats := make([]ContextBreakdownCategory, 0, len(order))
		for _, k := range order {
			v := catTotals[k]
			if v <= 0 {
				continue
			}
			pct := math.Round((float64(v)/float64(denom))*1000) / 10
			cats = append(cats, ContextBreakdownCategory{
				Key:     k,
				Tokens:  v,
				Percent: pct,
			})
		}

		sort.SliceStable(consumers, func(i, j int) bool {
			if consumers[i].Tokens != consumers[j].Tokens {
				return consumers[i].Tokens > consumers[j].Tokens
			}
			return consumers[i].StepIndex > consumers[j].StepIndex
		})
		if len(consumers) > topConsumerMaxItems {
			consumers = consumers[:topConsumerMaxItems]
		}

		if len(cats) > 0 || len(consumers) > 0 {
			out.Breakdown = &ContextBreakdownSummary{
				Categories:   cats,
				TopConsumers: consumers,
			}
		}
	}

	return &out
}
