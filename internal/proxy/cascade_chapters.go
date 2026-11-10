package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// CascadeStepRange specifies a half-open step interval [StartStepIndex, EndStepIndexExclusive)
// for Antigravity 2.30.0 GetChapterTitles.
type CascadeStepRange struct {
	StartStepIndex        int `json:"startStepIndex"`
	EndStepIndexExclusive int `json:"endStepIndexExclusive"`
}

// CascadeChapterTitle is one AI-generated semantic title for a step range.
type CascadeChapterTitle struct {
	StartStepIndex        int    `json:"startStepIndex"`
	EndStepIndexExclusive int    `json:"endStepIndexExclusive"`
	Title                 string `json:"title"`
	GeneratedAt           string `json:"generatedAt,omitempty"`
}

// CascadeChaptersRequest is the request body of POST /gateway/cascade/chapters.
type CascadeChaptersRequest struct {
	CascadeID string             `json:"cascadeId"`
	Ranges    []CascadeStepRange `json:"ranges"`
}

// CascadeChaptersResponse is the response body of POST /gateway/cascade/chapters.
type CascadeChaptersResponse struct {
	Chapters []CascadeChapterTitle `json:"chapters"`
}

var (
	chapterPollInterval = 350 * time.Millisecond
	chapterPollMaxWait  = 2800 * time.Millisecond
)

func cachedTrajectoryStepCount(cascadeID string) int {
	if cascadeID == "" {
		return 0
	}
	defaultTrajCache.trajCacheMu.RLock()
	defer defaultTrajCache.trajCacheMu.RUnlock()
	if entry, ok := defaultTrajCache.trajCache[cascadeID]; ok && entry != nil && entry.data != nil {
		return len(entry.data.Trajectory.Steps)
	}
	return 0
}

func (p *Proxy) callUpstreamGetChapterTitles(ctx context.Context, cascadeID string, ranges []CascadeStepRange, port int, token string) ([]CascadeChapterTitle, error) {
	payload, err := json.Marshal(map[string]interface{}{
		"cascadeId": cascadeID,
		"ranges":    ranges,
	})
	if err != nil {
		return nil, err
	}

	rpcURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetChapterTitles", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rpcURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	client := p.mediumClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		return nil, fmt.Errorf("GetChapterTitles returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Chapters []CascadeChapterTitle `json:"chapters"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}

	out := make([]CascadeChapterTitle, 0, len(parsed.Chapters))
	for _, ch := range parsed.Chapters {
		ch.Title = strings.TrimSpace(ch.Title)
		if ch.Title == "" || ch.EndStepIndexExclusive <= ch.StartStepIndex {
			continue
		}
		out = append(out, ch)
	}
	return out, nil
}

func (p *Proxy) resolveTrajectoryStepCount(ctx context.Context, cascadeID string, port int, token string) int {
	if n := cachedTrajectoryStepCount(cascadeID); n > 0 {
		return n
	}
	if traj, err := p.fetchUpstreamTrajectoryWithContext(ctx, cascadeID, port, token); err == nil && traj != nil {
		return len(traj.Trajectory.Steps)
	}
	return 0
}

// GetCascadeChapters fetches AI-generated chapter titles from Antigravity 2.30.0 GetChapterTitles.
// Because language_server generates uncached chapter titles asynchronously on the first call,
// this method polls loopback briefly until all requested ranges are resolved or the poll window expires.
func (p *Proxy) GetCascadeChapters(ctx context.Context, cascadeID string, ranges []CascadeStepRange, port int, token string) (*CascadeChaptersResponse, error) {
	stepCount := p.resolveTrajectoryStepCount(ctx, cascadeID, port, token)

	validRanges := make([]CascadeStepRange, 0, len(ranges))
	seen := make(map[string]bool, len(ranges))
	for _, r := range ranges {
		start := r.StartStepIndex
		if start < 0 {
			start = 0
		}
		end := r.EndStepIndexExclusive
		if stepCount > 0 && (end <= 0 || end > stepCount) {
			end = stepCount
		}
		if end <= start {
			continue
		}
		key := fmt.Sprintf("%d:%d", start, end)
		if seen[key] {
			continue
		}
		seen[key] = true
		validRanges = append(validRanges, CascadeStepRange{
			StartStepIndex:        start,
			EndStepIndexExclusive: end,
		})
	}

	if len(validRanges) == 0 {
		return &CascadeChaptersResponse{Chapters: []CascadeChapterTitle{}}, nil
	}

	chapters, err := p.callUpstreamGetChapterTitles(ctx, cascadeID, validRanges, port, token)
	if err != nil {
		return nil, err
	}

	if len(chapters) < len(validRanges) && chapterPollMaxWait > 0 && chapterPollInterval > 0 {
		deadline := time.Now().Add(chapterPollMaxWait)
		ticker := time.NewTicker(chapterPollInterval)
		defer ticker.Stop()

		for len(chapters) < len(validRanges) && time.Now().Before(deadline) {
			select {
			case <-ctx.Done():
				return &CascadeChaptersResponse{Chapters: chapters}, nil
			case <-ticker.C:
				next, pollErr := p.callUpstreamGetChapterTitles(ctx, cascadeID, validRanges, port, token)
				if pollErr == nil && len(next) >= len(chapters) {
					chapters = next
				}
			}
		}
	}

	if chapters == nil {
		chapters = []CascadeChapterTitle{}
	}
	return &CascadeChaptersResponse{Chapters: chapters}, nil
}

// HandleCascadeChapters handles POST /gateway/cascade/chapters.
func (p *Proxy) HandleCascadeChapters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CascadeChaptersRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 256*1024)).Decode(&req); err != nil {
		writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	req.CascadeID = strings.TrimSpace(req.CascadeID)
	if req.CascadeID == "" || !cascadeIDRe.MatchString(req.CascadeID) {
		writeJSONError(w, "cascadeId is required", http.StatusBadRequest)
		return
	}

	port, token := p.ActiveUpstream()
	if port == 0 {
		writeJSONError(w, "No active Antigravity upstream", http.StatusServiceUnavailable)
		return
	}

	resp, err := p.GetCascadeChapters(r.Context(), req.CascadeID, req.Ranges, port, token)
	if err != nil {
		// Degrade gracefully with empty chapters if upstream does not support GetChapterTitles yet
		writeGitJSON(w, CascadeChaptersResponse{Chapters: []CascadeChapterTitle{}})
		return
	}
	writeGitJSON(w, resp)
}
