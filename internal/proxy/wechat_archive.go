package proxy

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	maxWeChatArchiveTextBytes  = 16 * 1024 * 1024 // 16 MB limit for 聊天记录.txt
	maxWeChatZipEntryMemBytes  = 16 * 1024 * 1024 // 16 MB threshold to stream via temp cache file
	maxWeChatZipEntryReadBytes = 256 * 1024 * 1024
)

// WeChatChatArchive is the structured representation of a WeChat exported chat ZIP.
type WeChatChatArchive struct {
	IsWeChatArchive bool                `json:"is_wechat_archive"`
	Title           string              `json:"title"`
	DateRange       string              `json:"date_range"`
	IsMultiDay      bool                `json:"is_multi_day"`
	FileName        string              `json:"file_name"`
	MessageCount    int                 `json:"message_count"`
	RawText         string              `json:"raw_text,omitempty"`
	Messages        []WeChatChatMessage `json:"messages"`
	Entries         []WeChatZipFileInfo `json:"entries,omitempty"`
}

// WeChatZipFileInfo describes a file inside a ZIP archive (also used for fallback listing).
type WeChatZipFileInfo struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size uint64 `json:"size"`
}

// WeChatChatMessage represents a single parsed chat message in a WeChat archive.
type WeChatChatMessage struct {
	ID            int    `json:"id"`
	Sender        string `json:"sender"`
	SenderInitial string `json:"sender_initial"`
	ColorIndex    int    `json:"color_index"`
	Timestamp     string `json:"timestamp"`
	DisplayTime   string `json:"display_time"`
	Kind          string `json:"kind"` // text, image, video, file, sticker, voice, missing_image, missing_video, missing_file, link
	Text          string `json:"text"`
	MediaEntry    string `json:"media_entry,omitempty"`
	MediaName     string `json:"media_name,omitempty"`
	MediaSize     uint64 `json:"media_size,omitempty"`
	DurationLabel string `json:"duration_label,omitempty"`
}

var (
	weChatTimestampRe = regexp.MustCompile(`^(\d{4})年(\d{1,2})月(\d{1,2})日\s+(\d{1,2}):(\d{2})(?::(\d{2}))?$`)
	weChatTagRe       = regexp.MustCompile(`^\[(图片|视频|文件|动画表情|语音|链接|聊天记录)\](?:\s*(.*))?$`)
)

// IsWeChatArchiveName reports whether a file name matches WeChat's chat-history ZIP naming pattern.
func IsWeChatArchiveName(name string) bool {
	base := strings.TrimSpace(filepath.Base(name))
	if base == "" {
		return false
	}
	lower := strings.ToLower(base)
	return strings.HasPrefix(base, "聊天记录") && strings.HasSuffix(lower, ".zip")
}

func normalizeZipEntryPath(name string) string {
	clean := strings.ReplaceAll(name, "\\", "/")
	clean = strings.TrimPrefix(clean, "./")
	clean = path.Clean(clean)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return ""
	}
	return clean
}

func senderInitial(sender string) string {
	s := strings.TrimSpace(sender)
	if s == "" {
		return "?"
	}
	for _, r := range s {
		if r == '.' || r == '-' || r == '_' {
			continue
		}
		return strings.ToUpper(string(r))
	}
	r, _ := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return "?"
	}
	return strings.ToUpper(string(r))
}

func senderColorIndex(sender string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(sender)))
	return int(h.Sum32() % 8)
}

func parseWeChatTime(line string) (time.Time, bool) {
	m := weChatTimestampRe.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return time.Time{}, false
	}
	year, _ := strconv.Atoi(m[1])
	month, _ := strconv.Atoi(m[2])
	day, _ := strconv.Atoi(m[3])
	hour, _ := strconv.Atoi(m[4])
	minute, _ := strconv.Atoi(m[5])
	sec := 0
	if m[6] != "" {
		sec, _ = strconv.Atoi(m[6])
	}
	if month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || minute > 59 || sec > 59 {
		return time.Time{}, false
	}
	return time.Date(year, time.Month(month), day, hour, minute, sec, 0, time.Local), true
}

func isWeChatSenderLine(line string) (string, bool) {
	trimmed := strings.TrimLeft(line, " \t\u00a0")
	if !strings.HasPrefix(trimmed, "·") {
		return "", false
	}
	sender := strings.TrimSpace(strings.TrimPrefix(trimmed, "·"))
	if sender == "" || sender == "[聊天记录]" {
		return "", false
	}
	return sender, true
}

func stripWeChatIndent(line string) string {
	if strings.HasPrefix(line, "    ") {
		return line[4:]
	}
	if strings.HasPrefix(line, "\t") {
		return line[1:]
	}
	return strings.TrimLeft(line, " ")
}

type zipEntryLookup struct {
	byPath map[string]*zip.File
	byBase map[string]*zip.File
	list   []WeChatZipFileInfo
}

func buildZipEntryLookup(files []*zip.File) zipEntryLookup {
	lookup := zipEntryLookup{
		byPath: make(map[string]*zip.File, len(files)),
		byBase: make(map[string]*zip.File, len(files)),
	}
	for _, f := range files {
		if f.FileInfo().IsDir() {
			continue
		}
		norm := normalizeZipEntryPath(f.Name)
		if norm == "" {
			continue
		}
		lookup.byPath[norm] = f
		base := path.Base(norm)
		if _, exists := lookup.byBase[base]; !exists {
			lookup.byBase[base] = f
		}
		lookup.byBase[strings.ToLower(base)] = f
		lookup.list = append(lookup.list, WeChatZipFileInfo{
			Path: norm,
			Name: base,
			Size: f.UncompressedSize64,
		})
	}
	return lookup
}

func (l zipEntryLookup) findMedia(ref string) (*zip.File, string) {
	clean := normalizeZipEntryPath(strings.TrimSpace(ref))
	if clean == "" {
		return nil, ""
	}
	if f, ok := l.byPath[clean]; ok {
		return f, normalizeZipEntryPath(f.Name)
	}
	base := path.Base(clean)
	if f, ok := l.byBase[base]; ok {
		return f, normalizeZipEntryPath(f.Name)
	}
	if f, ok := l.byBase[strings.ToLower(base)]; ok {
		return f, normalizeZipEntryPath(f.Name)
	}
	return nil, ""
}

func parseWeChatChatLog(rawText string, lookup zipEntryLookup, zipFileName string) (*WeChatChatArchive, error) {
	cleaned := strings.TrimPrefix(rawText, "\ufeff")
	cleaned = strings.ReplaceAll(cleaned, "\r\n", "\n")
	cleaned = strings.ReplaceAll(cleaned, "\r", "\n")
	lines := strings.Split(cleaned, "\n")

	hasBanner := false
	startIdx := 0
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if t == "·[聊天记录]" || t == "[聊天记录]" {
			hasBanner = true
			startIdx = i + 1
		} else {
			startIdx = i
		}
		break
	}

	type rawMsg struct {
		sender string
		ts     time.Time
		tsRaw  string
		lines  []string
	}
	var parsed []rawMsg
	var current *rawMsg

	for i := startIdx; i < len(lines); i++ {
		if i+1 < len(lines) {
			if sender, ok := isWeChatSenderLine(lines[i]); ok {
				if ts, okTS := parseWeChatTime(lines[i+1]); okTS {
					if current != nil {
						parsed = append(parsed, *current)
					}
					current = &rawMsg{
						sender: sender,
						ts:     ts,
						tsRaw:  strings.TrimSpace(lines[i+1]),
					}
					i++ // skip timestamp line
					continue
				}
			}
		}
		if current != nil {
			current.lines = append(current.lines, stripWeChatIndent(lines[i]))
		}
	}
	if current != nil {
		parsed = append(parsed, *current)
	}

	if !hasBanner && len(parsed) == 0 {
		return &WeChatChatArchive{
			IsWeChatArchive: false,
			Title:           zipFileName,
			FileName:        zipFileName,
			Entries:         lookup.list,
		}, nil
	}

	var minTime, maxTime time.Time
	for idx, m := range parsed {
		if idx == 0 || m.ts.Before(minTime) {
			minTime = m.ts
		}
		if idx == 0 || m.ts.After(maxTime) {
			maxTime = m.ts
		}
	}

	isMultiDay := false
	isCrossYear := false
	dateRange := ""
	if len(parsed) > 0 {
		minY, minM, minD := minTime.Date()
		maxY, maxM, maxD := maxTime.Date()
		isMultiDay = minY != maxY || minM != maxM || minD != maxD
		isCrossYear = minY != maxY
		if isMultiDay {
			dateRange = fmt.Sprintf("%d年%d月%d日 至 %d年%d月%d日", minY, int(minM), minD, maxY, int(maxM), maxD)
		} else {
			dateRange = fmt.Sprintf("%d年%d月%d日", minY, int(minM), minD)
		}
	}

	messages := make([]WeChatChatMessage, 0, len(parsed))
	for idx, m := range parsed {
		// Trim leading/trailing empty lines within a message block
		bodyLines := m.lines
		for len(bodyLines) > 0 && strings.TrimSpace(bodyLines[0]) == "" {
			bodyLines = bodyLines[1:]
		}
		for len(bodyLines) > 0 && strings.TrimSpace(bodyLines[len(bodyLines)-1]) == "" {
			bodyLines = bodyLines[:len(bodyLines)-1]
		}
		body := strings.Join(bodyLines, "\n")
		body = strings.ReplaceAll(body, "\u2005", " ")
		trimmedBody := strings.TrimSpace(body)

		displayTime := m.ts.Format("15:04")
		if isCrossYear {
			displayTime = fmt.Sprintf("%d年%d月%d日 %s", m.ts.Year(), int(m.ts.Month()), m.ts.Day(), m.ts.Format("15:04"))
		} else if isMultiDay {
			displayTime = fmt.Sprintf("%d月%d日 %s", int(m.ts.Month()), m.ts.Day(), m.ts.Format("15:04"))
		}

		msg := WeChatChatMessage{
			ID:            idx + 1,
			Sender:        m.sender,
			SenderInitial: senderInitial(m.sender),
			ColorIndex:    senderColorIndex(m.sender),
			Timestamp:     m.ts.Format("2006-01-02 15:04"),
			DisplayTime:   displayTime,
			Kind:          "text",
			Text:          trimmedBody,
		}

		if tagMatch := weChatTagRe.FindStringSubmatch(trimmedBody); tagMatch != nil {
			tagType := tagMatch[1]
			arg := strings.TrimSpace(tagMatch[2])
			switch tagType {
			case "图片":
				if arg != "" {
					if f, norm := lookup.findMedia(arg); f != nil {
						msg.Kind = "image"
						msg.MediaEntry = norm
						msg.MediaName = path.Base(norm)
						msg.MediaSize = f.UncompressedSize64
						msg.Text = ""
					} else {
						msg.Kind = "missing_image"
						msg.MediaName = arg
						msg.Text = "[图片]"
					}
				} else {
					msg.Kind = "missing_image"
					msg.Text = "[图片]"
				}
			case "视频":
				if arg != "" {
					if f, norm := lookup.findMedia(arg); f != nil {
						msg.Kind = "video"
						msg.MediaEntry = norm
						msg.MediaName = path.Base(norm)
						msg.MediaSize = f.UncompressedSize64
						msg.Text = ""
					} else {
						msg.Kind = "missing_video"
						msg.MediaName = arg
						msg.Text = "[视频]"
					}
				} else {
					msg.Kind = "missing_video"
					msg.Text = "[视频]"
				}
			case "文件":
				if arg != "" {
					if f, norm := lookup.findMedia(arg); f != nil {
						msg.Kind = "file"
						msg.MediaEntry = norm
						msg.MediaName = path.Base(norm)
						msg.MediaSize = f.UncompressedSize64
						msg.Text = msg.MediaName
					} else {
						msg.Kind = "missing_file"
						msg.MediaName = arg
						msg.Text = "[文件] " + arg
					}
				} else {
					msg.Kind = "missing_file"
					msg.Text = "[文件]"
				}
			case "动画表情":
				msg.Kind = "sticker"
				msg.Text = arg
			case "语音":
				msg.Kind = "voice"
				msg.Text = arg
			case "链接":
				msg.Kind = "link"
				msg.Text = arg
			case "聊天记录":
				msg.Kind = "text"
				if arg != "" {
					msg.Text = "[聊天记录] " + arg
				} else {
					msg.Text = "[聊天记录]"
				}
			}
		}

		messages = append(messages, msg)
	}

	return &WeChatChatArchive{
		IsWeChatArchive: true,
		Title:           "群聊的聊天记录",
		DateRange:       dateRange,
		IsMultiDay:      isMultiDay,
		FileName:        zipFileName,
		MessageCount:    len(messages),
		RawText:         strings.TrimSpace(cleaned),
		Messages:        messages,
		Entries:         lookup.list,
	}, nil
}

func findWeChatLogEntry(files []*zip.File) *zip.File {
	var best *zip.File
	for _, f := range files {
		if f.FileInfo().IsDir() {
			continue
		}
		norm := normalizeZipEntryPath(f.Name)
		if norm == "" {
			continue
		}
		base := path.Base(norm)
		if base == "聊天记录.txt" {
			return f
		}
		if strings.HasSuffix(strings.ToLower(base), ".txt") && best == nil {
			best = f
		}
	}
	return best
}

func ParseWeChatZipFile(zipPath string) (*WeChatChatArchive, error) {
	rc, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open zip archive: %w", err)
	}
	defer rc.Close()

	lookup := buildZipEntryLookup(rc.File)
	baseZipName := filepath.Base(zipPath)
	// Strip "<id16>-" prefix when displaying file name
	if len(baseZipName) > attachmentIDLen+1 && baseZipName[attachmentIDLen] == '-' && attachmentIDPattern.MatchString(baseZipName[:attachmentIDLen]) {
		baseZipName = baseZipName[attachmentIDLen+1:]
	}

	logEntry := findWeChatLogEntry(rc.File)
	if logEntry == nil {
		return &WeChatChatArchive{
			IsWeChatArchive: false,
			Title:           baseZipName,
			FileName:        baseZipName,
			Entries:         lookup.list,
		}, nil
	}

	r, err := logEntry.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to read chat log entry: %w", err)
	}
	defer r.Close()

	data, err := io.ReadAll(io.LimitReader(r, maxWeChatArchiveTextBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read chat log: %w", err)
	}
	if len(data) > maxWeChatArchiveTextBytes {
		return nil, fmt.Errorf("chat log exceeds size limit")
	}

	archive, err := parseWeChatChatLog(string(data), lookup, baseZipName)
	if err != nil {
		return nil, err
	}

	// Best-effort probe of video durations when ffprobe is present (bounded count & size so large archives open fast).
	const maxSyncProbedVideos = 8
	const maxSyncProbeEntryBytes = 64 * 1024 * 1024
	probedVideos := 0
	probeDeadline := time.Now().Add(4 * time.Second)
	for i := range archive.Messages {
		if probedVideos >= maxSyncProbedVideos || time.Now().After(probeDeadline) {
			break
		}
		if archive.Messages[i].Kind == "video" && archive.Messages[i].MediaEntry != "" {
			if f, _ := lookup.findMedia(archive.Messages[i].MediaEntry); f != nil && f.UncompressedSize64 <= maxSyncProbeEntryBytes {
				probedVideos++
				if dur := probeZipVideoDuration(zipPath, f); dur != "" {
					archive.Messages[i].DurationLabel = dur
				}
			}
		}
	}

	return archive, nil
}

const (
	defaultZipCacheTTL      = 6 * time.Hour
	defaultZipCacheMaxBytes = 256 * 1024 * 1024
	mediaSubprocessTimeout  = 5 * time.Second
)

var (
	zipMaterializeFlights flightGroup[string]
	zipMediaSem           = make(chan struct{}, 2)
	zipPruneMu            sync.Mutex
	lastZipPruneAt        time.Time
)

func zipEntryCacheDir() string {
	return filepath.Join(os.TempDir(), "mgy-zip-cache")
}

// CleanupZipEntryCache evicts expired entries (older than ttl), stale temp files, and oldest LRU files
// when total cache usage exceeds maxTotalBytes, protecting host disk space.
func CleanupZipEntryCache(dir string, ttl time.Duration, maxTotalBytes int64) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	now := time.Now()
	cutoff := now.Add(-ttl)
	tmpCutoff := now.Add(-10 * time.Minute)

	type cacheItem struct {
		path    string
		size    int64
		modTime time.Time
	}
	var kept []cacheItem
	var totalBytes int64
	removed := 0

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fullPath := filepath.Join(dir, e.Name())
		info, iErr := e.Info()
		if iErr != nil {
			continue
		}
		isTmp := strings.Contains(e.Name(), ".tmp")
		if (isTmp && info.ModTime().Before(tmpCutoff)) || (ttl > 0 && info.ModTime().Before(cutoff)) {
			if os.Remove(fullPath) == nil {
				removed++
			}
			continue
		}
		if !isTmp {
			kept = append(kept, cacheItem{
				path:    fullPath,
				size:    info.Size(),
				modTime: info.ModTime(),
			})
			totalBytes += info.Size()
		}
	}

	if maxTotalBytes > 0 && totalBytes > maxTotalBytes {
		sort.Slice(kept, func(i, j int) bool {
			return kept[i].modTime.Before(kept[j].modTime)
		})
		for _, it := range kept {
			if totalBytes <= maxTotalBytes {
				break
			}
			if os.Remove(it.path) == nil {
				totalBytes -= it.size
				removed++
			}
		}
	}
	return removed
}

func maybePruneZipEntryCacheAsync() {
	zipPruneMu.Lock()
	if time.Since(lastZipPruneAt) < 5*time.Minute {
		zipPruneMu.Unlock()
		return
	}
	lastZipPruneAt = time.Now()
	zipPruneMu.Unlock()

	go func() {
		_ = CleanupZipEntryCache(zipEntryCacheDir(), defaultZipCacheTTL, defaultZipCacheMaxBytes)
	}()
}

func materializeZipEntryFile(zipPath string, f *zip.File) (string, error) {
	if f.UncompressedSize64 > maxWeChatZipEntryReadBytes {
		return "", fmt.Errorf("zip entry exceeds size limit")
	}
	st, err := os.Stat(zipPath)
	if err != nil {
		return "", err
	}
	keySrc := fmt.Sprintf("%s|%d|%d|%s|%d", zipPath, st.Size(), st.ModTime().UnixNano(), f.Name, f.CRC32)
	sum := sha256.Sum256([]byte(keySrc))
	key := hex.EncodeToString(sum[:16])

	ext := strings.ToLower(filepath.Ext(f.Name))
	if len(ext) > 16 {
		ext = ""
	}
	dir := zipEntryCacheDir()
	cachePath := filepath.Join(dir, key+ext)

	if info, err := os.Stat(cachePath); err == nil && info.Mode().IsRegular() && uint64(info.Size()) == f.UncompressedSize64 {
		now := time.Now()
		_ = os.Chtimes(cachePath, now, now)
		return cachePath, nil
	}

	return zipMaterializeFlights.do(cachePath, func() (string, error) {
		if info, err := os.Stat(cachePath); err == nil && info.Mode().IsRegular() && uint64(info.Size()) == f.UncompressedSize64 {
			return cachePath, nil
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", err
		}
		tmpPath := fmt.Sprintf("%s.tmp.%d", cachePath, time.Now().UnixNano())
		src, err := f.Open()
		if err != nil {
			return "", err
		}
		defer src.Close()

		dst, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return "", err
		}
		n, copyErr := io.Copy(dst, io.LimitReader(src, maxWeChatZipEntryReadBytes+1))
		closeErr := dst.Close()
		if copyErr != nil || closeErr != nil {
			_ = os.Remove(tmpPath)
			if copyErr != nil {
				return "", copyErr
			}
			return "", closeErr
		}
		if n > maxWeChatZipEntryReadBytes {
			_ = os.Remove(tmpPath)
			return "", fmt.Errorf("zip entry exceeds size limit")
		}
		if err := os.Rename(tmpPath, cachePath); err != nil {
			_ = os.Remove(tmpPath)
			return "", err
		}
		maybePruneZipEntryCacheAsync()
		return cachePath, nil
	})
}

func findMediaBinary(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, candidate := range []string{"/opt/homebrew/bin/" + name, "/usr/local/bin/" + name, "/usr/bin/" + name} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

var ffmpegDurationRe = regexp.MustCompile(`Duration:\s*(\d{2}):(\d{2}):(\d{2}(?:\.\d+)?)`)

func probeZipVideoDuration(zipPath string, f *zip.File) string {
	localPath, err := materializeZipEntryFile(zipPath, f)
	if err != nil {
		return ""
	}
	zipMediaSem <- struct{}{}
	defer func() { <-zipMediaSem }()

	if ffprobeBin := findMediaBinary("ffprobe"); ffprobeBin != "" {
		ctx, cancel := context.WithTimeout(context.Background(), mediaSubprocessTimeout)
		cmd := exec.CommandContext(ctx, ffprobeBin, "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", localPath)
		out, err := cmd.Output()
		cancel()
		if err == nil {
			if secs, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64); err == nil && secs > 0 {
				total := int(secs + 0.5)
				if total < 1 {
					total = 1
				}
				return fmt.Sprintf("%d:%02d", total/60, total%60)
			}
		}
	}
	if ffmpegBin := findMediaBinary("ffmpeg"); ffmpegBin != "" {
		ctx, cancel := context.WithTimeout(context.Background(), mediaSubprocessTimeout)
		cmd := exec.CommandContext(ctx, ffmpegBin, "-i", localPath)
		out, _ := cmd.CombinedOutput()
		cancel()
		if m := ffmpegDurationRe.FindStringSubmatch(string(out)); m != nil {
			h, _ := strconv.Atoi(m[1])
			min, _ := strconv.Atoi(m[2])
			secF, _ := strconv.ParseFloat(m[3], 64)
			total := h*3600 + min*60 + int(secF+0.5)
			if total < 1 {
				total = 1
			}
			return fmt.Sprintf("%d:%02d", total/60, total%60)
		}
	}
	return ""
}

func generateZipVideoThumb(zipPath string, f *zip.File) (string, error) {
	ffmpegBin := findMediaBinary("ffmpeg")
	if ffmpegBin == "" {
		return "", errors.New("ffmpeg unavailable")
	}
	localPath, err := materializeZipEntryFile(zipPath, f)
	if err != nil {
		return "", err
	}
	thumbPath := localPath + ".thumb.jpg"
	if info, err := os.Stat(thumbPath); err == nil && info.Size() > 0 {
		return thumbPath, nil
	}
	zipMediaSem <- struct{}{}
	defer func() { <-zipMediaSem }()

	if info, err := os.Stat(thumbPath); err == nil && info.Size() > 0 {
		return thumbPath, nil
	}
	tmpThumb := fmt.Sprintf("%s.tmp.%d.jpg", thumbPath, time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), mediaSubprocessTimeout)
	cmd := exec.CommandContext(ctx, ffmpegBin, "-y", "-ss", "0.1", "-i", localPath, "-vframes", "1", "-vf", "scale='min(480,iw)':-2", "-q:v", "4", tmpThumb)
	runErr := cmd.Run()
	cancel()
	if runErr != nil {
		_ = os.Remove(tmpThumb)
		return "", runErr
	}
	if err := os.Rename(tmpThumb, thumbPath); err != nil {
		_ = os.Remove(tmpThumb)
		return "", err
	}
	return thumbPath, nil
}

func resolveSafeZipPath(uri, cascadeID string) (string, int, error) {
	if uri == "" && cascadeID == "" {
		return "", http.StatusBadRequest, errors.New("uri or cascade_id is required")
	}
	filePath, err := ResolveLocalFilePath(uri, cascadeID)
	if err != nil {
		if errors.Is(err, errAccessRestricted) {
			return "", http.StatusForbidden, errors.New("access to file is restricted")
		}
		return "", http.StatusBadRequest, fmt.Errorf("failed to resolve file path: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(filePath); err == nil {
		filePath = resolved
	}
	if !IsSafeReadableFilePath(filePath) {
		return "", http.StatusForbidden, errors.New("access to file is restricted")
	}
	st, err := os.Stat(filePath)
	if err != nil || st.IsDir() || !st.Mode().IsRegular() {
		return "", http.StatusNotFound, errors.New("file not found")
	}
	return filePath, http.StatusOK, nil
}

// HandleWeChatChatArchive handles GET /api/v1/files/wechat-chat?uri=...&cascade_id=...
func (p *Proxy) HandleWeChatChatArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	uri := strings.TrimSpace(q.Get("uri"))
	cascadeID := strings.TrimSpace(q.Get("cascade_id"))

	filePath, status, err := resolveSafeZipPath(uri, cascadeID)
	if err != nil {
		writeAttachmentError(w, status, "invalid_file", err.Error())
		return
	}

	archive, err := ParseWeChatZipFile(filePath)
	if err != nil {
		writeAttachmentError(w, http.StatusUnprocessableEntity, "parse_failed", err.Error())
		return
	}

	data, err := json.Marshal(archive)
	if err != nil {
		writeAttachmentError(w, http.StatusInternalServerError, "encode_failed", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// HandleZipEntry handles GET /api/v1/files/zip-entry?uri=...&entry=...&thumb=1
func (p *Proxy) HandleZipEntry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	uri := strings.TrimSpace(q.Get("uri"))
	cascadeID := strings.TrimSpace(q.Get("cascade_id"))
	entryParam := normalizeZipEntryPath(strings.TrimSpace(q.Get("entry")))
	wantThumb := q.Get("thumb") == "1" || q.Get("thumb") == "true"

	if entryParam == "" {
		http.Error(w, "entry is required", http.StatusBadRequest)
		return
	}

	filePath, status, err := resolveSafeZipPath(uri, cascadeID)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}

	rc, err := zip.OpenReader(filePath)
	if err != nil {
		http.Error(w, "failed to open zip archive", http.StatusUnprocessableEntity)
		return
	}
	defer rc.Close()

	lookup := buildZipEntryLookup(rc.File)
	entryFile, normPath := lookup.findMedia(entryParam)
	if entryFile == nil {
		http.Error(w, "entry not found", http.StatusNotFound)
		return
	}

	if wantThumb {
		thumbPath, err := generateZipVideoThumb(filePath, entryFile)
		if err != nil {
			http.Error(w, "thumbnail unavailable", http.StatusNotFound)
			return
		}
		tf, err := os.Open(thumbPath)
		if err != nil {
			http.Error(w, "thumbnail unavailable", http.StatusNotFound)
			return
		}
		defer tf.Close()
		tfi, err := tf.Stat()
		if err != nil {
			http.Error(w, "thumbnail unavailable", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "private, max-age=604800")
		http.ServeContent(w, r, path.Base(normPath)+".jpg", tfi.ModTime(), tf)
		return
	}

	baseName := path.Base(normPath)
	ext := strings.ToLower(path.Ext(baseName))
	if mimeType := mimeForName(baseName); mimeType != "" {
		w.Header().Set("Content-Type", mimeType)
	}
	switch ext {
	case ".html", ".htm", ".svg", ".xml", ".xhtml", ".mhtml":
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q; filename*=UTF-8''%s", baseName, url.PathEscape(baseName)))
	default:
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q; filename*=UTF-8''%s", baseName, url.PathEscape(baseName)))
	}
	w.Header().Set("Cache-Control", "private, max-age=604800, must-revalidate")

	if entryFile.UncompressedSize64 <= maxWeChatZipEntryMemBytes {
		src, err := entryFile.Open()
		if err != nil {
			http.Error(w, "failed to open zip entry", http.StatusInternalServerError)
			return
		}
		defer src.Close()
		payload, err := io.ReadAll(io.LimitReader(src, maxWeChatZipEntryMemBytes+1))
		if err != nil {
			http.Error(w, "failed to read zip entry", http.StatusInternalServerError)
			return
		}
		http.ServeContent(w, r, baseName, entryFile.Modified, bytes.NewReader(payload))
		return
	}

	localPath, err := materializeZipEntryFile(filePath, entryFile)
	if err != nil {
		http.Error(w, "failed to extract zip entry", http.StatusInternalServerError)
		return
	}
	lf, err := os.Open(localPath)
	if err != nil {
		http.Error(w, "failed to read zip entry", http.StatusInternalServerError)
		return
	}
	defer lf.Close()
	lfi, err := lf.Stat()
	if err != nil {
		http.Error(w, "failed to stat zip entry", http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, baseName, lfi.ModTime(), lf)
}
