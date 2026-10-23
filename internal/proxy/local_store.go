package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// AntigravityLocalStore provides direct, zero-RPC read access to Antigravity's local SQLite database
// (conversation_summaries.db) introduced in Antigravity 2.26+. It supports:
// 1. Lightning-fast global FTS5 full-text search across conversations, steps, and workspace files.
// 2. Zero-RPC conversation summaries fallback and metadata hydration (including parent-child hierarchy).

type SearchMatchRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type ConversationSearchResult struct {
	CascadeID          string             `json:"cascadeId"`
	Title              string             `json:"title"`
	WorkspaceName      string             `json:"workspaceName,omitempty"`
	LastModifiedTime   string             `json:"lastModifiedTime,omitempty"`
	Snippet            string             `json:"snippet"`
	SnippetMatchRanges []SearchMatchRange `json:"snippetMatchRanges"`
	MatchSource        string             `json:"matchSource,omitempty"`
	MatchedStepIndex   int                `json:"matchedStepIndex,omitempty"`
}

type ConversationSearchResponse struct {
	Results []ConversationSearchResult `json:"results"`
}

type LocalConversationSummary struct {
	ConversationID       string `json:"conversationId"`
	Title                string `json:"title"`
	Preview              string `json:"preview"`
	StepCount            int    `json:"stepCount"`
	LastModifiedTime     string `json:"lastModifiedTime"`
	WorkspaceURIs        string `json:"workspaceUris"`
	Status               string `json:"status"`
	ParentConversationID string `json:"parentConversationId,omitempty"`
	NestingDepth         int    `json:"nestingDepth,omitempty"`
}

var (
	localStoreMu       sync.RWMutex
	cachedSearchQuery  string
	cachedSearchResp   []byte
	cachedSearchAt     time.Time
	searchCacheTTL     = 2 * time.Second

	cachedSummariesMap map[string]map[string]interface{}
	cachedSummariesAt  time.Time
	summariesCacheTTL  = 3 * time.Second
)

// getAntigravityDBPath returns the absolute path to conversation_summaries.db if it exists.
func getAntigravityDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	p := filepath.Join(home, ".gemini", "antigravity", "conversation_summaries.db")
	if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
		return p
	}
	return ""
}

// SearchConversationsFTS5 performs a full-text search directly against Antigravity's conversations_fts virtual table.
func SearchConversationsFTS5(ctx context.Context, query string, limit int) ([]ConversationSearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	// Fast cache check
	localStoreMu.RLock()
	if cachedSearchQuery == query && time.Since(cachedSearchAt) < searchCacheTTL && len(cachedSearchResp) > 0 {
		var resp ConversationSearchResponse
		if err := json.Unmarshal(cachedSearchResp, &resp); err == nil {
			localStoreMu.RUnlock()
			return resp.Results, nil
		}
	}
	localStoreMu.RUnlock()

	dbPath := getAntigravityDBPath()
	if dbPath == "" {
		return nil, fmt.Errorf("antigravity conversation database not found")
	}

	pyBin, err := findPythonExecutable()
	if err != nil {
		return nil, fmt.Errorf("python runtime required for SQLite FTS5 query: %w", err)
	}

	pyScript := `
import os, sqlite3, json, sys

db_path = sys.argv[1]
query = sys.argv[2]
limit = int(sys.argv[3])

try:
    conn = sqlite3.connect(f'file:{db_path}?mode=ro', uri=True)
except Exception:
    conn = sqlite3.connect(db_path)

cur = conn.cursor()
sql = '''
SELECT f.conversation_id, COALESCE(NULLIF(c.title, ''), f.title), f.workspace, 
       snippet(conversations_fts, 13, '<b>', '</b>', '...', 12),
       f.chunk_first_step, c.last_modified_time
FROM conversations_fts f
LEFT JOIN conversation_summaries c ON f.conversation_id = c.conversation_id
WHERE conversations_fts MATCH ?
ORDER BY rank
LIMIT ?
'''
cur.execute(sql, (query, limit))
rows = cur.fetchall()
results = []
for r in rows:
    cid, title, ws, snip, step_idx, mod_time = r
    clean_snip = snip.replace('<b>', '').replace('</b>', '')
    ranges = []
    curr = snip
    offset = 0
    while '<b>' in curr:
        b_start = curr.find('<b>')
        b_end = curr.find('</b>', b_start)
        if b_end == -1: break
        start_idx = len(curr[:b_start].replace('<b>', '').replace('</b>', '')) + offset
        match_len = len(curr[b_start+3:b_end])
        ranges.append({'start': start_idx, 'end': start_idx + match_len})
        offset += len(curr[:b_end+4].replace('<b>', '').replace('</b>', ''))
        curr = curr[b_end+4:]
        
    results.append({
        'cascadeId': cid,
        'title': title or '未命名会话',
        'workspaceName': ws or '',
        'lastModifiedTime': str(mod_time) if mod_time else '',
        'snippet': clean_snip,
        'snippetMatchRanges': ranges,
        'matchSource': 'conversation_fts',
        'matchedStepIndex': step_idx or 0
    })
print(json.dumps({'results': results}, ensure_ascii=False))
`

	execCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, pyBin, "-c", pyScript, dbPath, query, fmt.Sprintf("%d", limit))
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		slog.Warn("[LocalStore] FTS5 query failed", "err", err, "stderr", errBuf.String())
		return nil, fmt.Errorf("fts5 execution error: %v (%s)", err, errBuf.String())
	}

	var searchResp ConversationSearchResponse
	if err := json.Unmarshal(outBuf.Bytes(), &searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode search json: %w", err)
	}

	// Update cache
	localStoreMu.Lock()
	cachedSearchQuery = query
	cachedSearchResp = outBuf.Bytes()
	cachedSearchAt = time.Now()
	localStoreMu.Unlock()

	return searchResp.Results, nil
}

// FetchLocalConversationSummaries retrieves conversation summaries directly from conversation_summaries.db.
// It returns a map compatible with LanguageServer's GetAllCascadeTrajectories response.
func FetchLocalConversationSummaries(ctx context.Context) (map[string]map[string]interface{}, error) {
	localStoreMu.RLock()
	if cachedSummariesMap != nil && time.Since(cachedSummariesAt) < summariesCacheTTL {
		res := cachedSummariesMap
		localStoreMu.RUnlock()
		return res, nil
	}
	localStoreMu.RUnlock()

	dbPath := getAntigravityDBPath()
	if dbPath == "" {
		return nil, fmt.Errorf("antigravity conversation database not found")
	}

	pyBin, err := findPythonExecutable()
	if err != nil {
		return nil, fmt.Errorf("python runtime required: %w", err)
	}

	pyScript := `
import os, sqlite3, json, sys

db_path = sys.argv[1]
try:
    conn = sqlite3.connect(f'file:{db_path}?mode=ro', uri=True)
except Exception:
    conn = sqlite3.connect(db_path)

cur = conn.cursor()
sql = '''
SELECT conversation_id, title, preview, step_count, last_modified_time, 
       workspace_uris, status, parent_conversation_id, nesting_depth
FROM conversation_summaries
ORDER BY last_modified_time DESC
'''
cur.execute(sql)
rows = cur.fetchall()
summaries = {}
for r in rows:
    cid, title, preview, step_count, mod_time, ws_uris, status, parent_id, depth = r
    status_str = "CASCADE_RUN_STATUS_COMPLETED"
    if status == "running" or status == "CASCADE_RUN_STATUS_RUNNING":
        status_str = "CASCADE_RUN_STATUS_RUNNING"
    elif status == "error" or status == "CASCADE_RUN_STATUS_ERROR":
        status_str = "CASCADE_RUN_STATUS_ERROR"
        
    summaries[cid] = {
        "cascadeId": cid,
        "status": status_str,
        "lastModifiedTime": str(mod_time) if mod_time else "",
        "stepCount": step_count or 0,
        "summary": preview or "",
        "parentConversationId": parent_id or "",
        "nestingDepth": depth or 0,
        "trajectory": {
            "cascadeId": cid,
            "summary": preview or "",
            "annotations": {
                "title": title or "未命名会话"
            }
        }
    }
print(json.dumps(summaries, ensure_ascii=False))
`

	execCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, pyBin, "-c", pyScript, dbPath)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		slog.Warn("[LocalStore] Fetch summaries failed", "err", err, "stderr", errBuf.String())
		return nil, fmt.Errorf("fetch summaries error: %v (%s)", err, errBuf.String())
	}

	var res map[string]map[string]interface{}
	if err := json.Unmarshal(outBuf.Bytes(), &res); err != nil {
		return nil, fmt.Errorf("failed to decode summaries json: %w", err)
	}

	localStoreMu.Lock()
	cachedSummariesMap = res
	cachedSummariesAt = time.Now()
	localStoreMu.Unlock()

	return res, nil
}

func findPythonExecutable() (string, error) {
	for _, bin := range []string{"python3", "python"} {
		if path, err := exec.LookPath(bin); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("neither python3 nor python found in PATH")
}
