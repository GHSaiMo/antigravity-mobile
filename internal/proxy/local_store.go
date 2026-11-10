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
	"runtime"
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

	cachedSummariesMap   map[string]map[string]interface{}
	cachedSummariesAt    time.Time
	cachedSummariesStamp string
	summariesCacheTTL    = 3 * time.Second
	summariesFlights     flightGroup[map[string]map[string]interface{}]
)

func sqliteDBStamp(dbPath string) string {
	fi, err := os.Stat(dbPath)
	if err != nil {
		return ""
	}
	var walNano, walSize int64
	if wfi, werr := os.Stat(dbPath + "-wal"); werr == nil {
		walNano = wfi.ModTime().UnixNano()
		walSize = wfi.Size()
	}
	return fmt.Sprintf("%s|%d|%d|%d|%d", dbPath, fi.ModTime().UnixNano(), fi.Size(), walNano, walSize)
}

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
import os, sqlite3, json, sys, time

db_path = sys.argv[1]
query = sys.argv[2]
limit = int(sys.argv[3])

def open_db(path):
    last_err = None
    for attempt in range(3):
        for uri_flag, conn_str in [(True, f'file:{path}?mode=ro'), (False, path)]:
            conn = None
            try:
                conn = sqlite3.connect(conn_str, uri=uri_flag, timeout=3.0)
                try:
                    conn.execute('PRAGMA busy_timeout = 2500')
                    if not uri_flag:
                        conn.execute('PRAGMA query_only = ON')
                except Exception:
                    pass
                test_cur = conn.cursor()
                test_cur.execute('SELECT 1 FROM sqlite_master LIMIT 1')
                test_cur.close()
                return conn
            except Exception as e:
                last_err = e
                if conn:
                    try:
                        conn.close()
                    except Exception:
                        pass
        time.sleep(0.1)
    raise last_err or sqlite3.OperationalError(f'unable to open database file: {path}')

conn = open_db(db_path)
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
try:
    cur.execute(sql, (query, limit))
except sqlite3.OperationalError:
    safe_query = '"' + query.replace('"', '""') + '"'
    cur.execute(sql, (safe_query, limit))
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
sys.stdout.buffer.write(json.dumps({'results': results}, ensure_ascii=False).encode('utf-8'))
`

	execCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, pyBin, "-c", pyScript, dbPath, query, fmt.Sprintf("%d", limit))
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")
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
	fallbackCache := cachedSummariesMap
	prevStamp := cachedSummariesStamp
	localStoreMu.RUnlock()

	dbPath := getAntigravityDBPath()
	if dbPath == "" {
		if fallbackCache != nil {
			return fallbackCache, nil
		}
		return nil, fmt.Errorf("antigravity conversation database not found")
	}

	curStamp := sqliteDBStamp(dbPath)
	if fallbackCache != nil && curStamp != "" && curStamp == prevStamp {
		localStoreMu.Lock()
		if cachedSummariesMap != nil && cachedSummariesStamp == curStamp {
			cachedSummariesAt = time.Now()
		}
		localStoreMu.Unlock()
		return fallbackCache, nil
	}

	return summariesFlights.do(dbPath, func() (map[string]map[string]interface{}, error) {
		stamp := sqliteDBStamp(dbPath)
		localStoreMu.RLock()
		if cachedSummariesMap != nil && (time.Since(cachedSummariesAt) < summariesCacheTTL || (stamp != "" && stamp == cachedSummariesStamp)) {
			res := cachedSummariesMap
			localStoreMu.RUnlock()
			return res, nil
		}
		fb := cachedSummariesMap
		localStoreMu.RUnlock()

		pyBin, err := findPythonExecutable()
		if err != nil {
			if fb != nil {
				return fb, nil
			}
			return nil, fmt.Errorf("python runtime required: %w", err)
		}

		pyScript := `
import os, sqlite3, json, sys, time

db_path = sys.argv[1]

def open_db(path):
    last_err = None
    for attempt in range(3):
        for uri_flag, conn_str in [(True, f'file:{path}?mode=ro'), (False, path)]:
            conn = None
            try:
                conn = sqlite3.connect(conn_str, uri=uri_flag, timeout=3.0)
                try:
                    conn.execute('PRAGMA busy_timeout = 2500')
                    if not uri_flag:
                        conn.execute('PRAGMA query_only = ON')
                except Exception:
                    pass
                test_cur = conn.cursor()
                test_cur.execute('SELECT 1 FROM sqlite_master LIMIT 1')
                test_cur.close()
                return conn
            except Exception as e:
                last_err = e
                if conn:
                    try:
                        conn.close()
                    except Exception:
                        pass
        time.sleep(0.1)
    raise last_err or sqlite3.OperationalError(f'unable to open database file: {path}')

conn = open_db(db_path)
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
sys.stdout.buffer.write(json.dumps(summaries, ensure_ascii=False).encode('utf-8'))
`

		execCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()

		cmd := exec.CommandContext(execCtx, pyBin, "-c", pyScript, dbPath)
		cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")
		var outBuf, errBuf bytes.Buffer
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf

		if err := cmd.Run(); err != nil {
			if fb != nil {
				slog.Debug("[LocalStore] Fetch summaries failed, serving stale cache", "err", err)
				return fb, nil
			}
			slog.Warn("[LocalStore] Fetch summaries failed", "err", err, "stderr", errBuf.String())
			return nil, fmt.Errorf("fetch summaries error: %v (%s)", err, errBuf.String())
		}

		var res map[string]map[string]interface{}
		if err := json.Unmarshal(outBuf.Bytes(), &res); err != nil {
			if fb != nil {
				return fb, nil
			}
			return nil, fmt.Errorf("failed to decode summaries json: %w", err)
		}

		finalStamp := sqliteDBStamp(dbPath)
		localStoreMu.Lock()
		cachedSummariesMap = res
		cachedSummariesAt = time.Now()
		cachedSummariesStamp = finalStamp
		localStoreMu.Unlock()

		return res, nil
	})
}

var (
	pyBinOnce   sync.Once
	cachedPyBin string
	cachedPyErr error
)

func isExecutableValid(path string) bool {
	resolved := path
	if !filepath.IsAbs(resolved) && !strings.ContainsRune(resolved, filepath.Separator) && !strings.ContainsRune(resolved, '/') {
		if p, err := exec.LookPath(resolved); err == nil {
			resolved = p
		}
	}
	if fi, err := os.Stat(resolved); err != nil || fi.Size() == 0 {
		return false
	}
	return true
}

func pythonCandidates() []string {
	if runtime.GOOS == "windows" {
		return []string{"python", "py", "python3"}
	}
	return []string{"python3", "python"}
}

func findPythonExecutable() (string, error) {
	pyBinOnce.Do(func() {
		for _, bin := range pythonCandidates() {
			if path, err := exec.LookPath(bin); err == nil && isExecutableValid(path) {
				// Verify candidate actually executes without crashing (filters out broken execution aliases / code 9009)
				cmd := exec.Command(bin, "-c", "import sys; sys.exit(0)")
				if cmdErr := cmd.Run(); cmdErr == nil {
					cachedPyBin = bin
					return
				}
			}
		}
		cachedPyErr = fmt.Errorf("neither python nor python3 found in PATH (or execution alias failed)")
	})
	return cachedPyBin, cachedPyErr
}
