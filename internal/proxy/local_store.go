package proxy

import (
	"bytes"
	"context"
	"encoding/hex"
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
	"unicode/utf8"

	"antigravity-mobile/internal/procutil"
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
	CascadeID            string             `json:"cascadeId"`
	Title                string             `json:"title"`
	WorkspaceName        string             `json:"workspaceName,omitempty"`
	LastModifiedTime     string             `json:"lastModifiedTime,omitempty"`
	Snippet              string             `json:"snippet"`
	SnippetMatchRanges   []SearchMatchRange `json:"snippetMatchRanges"`
	MatchSource          string             `json:"matchSource,omitempty"`
	MatchedStepIndex     int                `json:"matchedStepIndex,omitempty"`
	IsSideQuestion       bool               `json:"isSideQuestion,omitempty"`
	ParentConversationID string             `json:"parentConversationId,omitempty"`
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

type searchCacheEntry struct {
	results []ConversationSearchResult
	at      time.Time
}

var (
	localStoreMu   sync.RWMutex
	searchCache    = make(map[string]searchCacheEntry)
	searchCacheTTL = 3 * time.Second
	ftsSearchSem   = make(chan struct{}, 2)

	cachedSummariesMap   map[string]map[string]interface{}
	cachedSummariesAt    time.Time
	cachedSummariesStamp string
	summariesCacheTTL    = 3 * time.Second
	summariesFlights     flightGroup[map[string]map[string]interface{}]

	hierarchyMu    sync.RWMutex
	hierarchyCache = make(map[string]hierarchyMeta)
)

type hierarchyMeta struct {
	parentID string
	depth    int
}

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

// getAntigravityDBPath returns the absolute path to conversation_summaries.db if it exists,
// checking ~/.gemini/antigravity first and falling back to ~/.gemini/antigravity-cli.
func getAntigravityDBPath() string {
	p := resolveAntigravitySubPath("conversation_summaries.db")
	if p == "" {
		return ""
	}
	if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
		return p
	}
	return ""
}

const (
	ftsMarkerStart = "\x01MGYB\x02"
	ftsMarkerEnd   = "\x01MGYE\x02"
)

func parseFTSSnippetRanges(marked string) (string, []SearchMatchRange) {
	ranges := make([]SearchMatchRange, 0)
	var clean strings.Builder
	clean.Grow(len(marked))
	runeOffset := 0
	rem := marked
	for {
		bStart := strings.Index(rem, ftsMarkerStart)
		if bStart < 0 {
			clean.WriteString(rem)
			break
		}
		prefix := rem[:bStart]
		clean.WriteString(prefix)
		runeOffset += utf8.RuneCountInString(prefix)

		afterStart := rem[bStart+len(ftsMarkerStart):]
		bEnd := strings.Index(afterStart, ftsMarkerEnd)
		if bEnd < 0 {
			clean.WriteString(afterStart)
			break
		}
		matched := afterStart[:bEnd]
		matchLen := utf8.RuneCountInString(matched)
		clean.WriteString(matched)
		ranges = append(ranges, SearchMatchRange{
			Start: runeOffset,
			End:   runeOffset + matchLen,
		})
		runeOffset += matchLen
		rem = afterStart[bEnd+len(ftsMarkerEnd):]
	}
	return clean.String(), ranges
}

func safeSQLiteDBArg(dbPath string) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(dbPath))
	if clean == "" || clean == "." || strings.ContainsAny(clean, "\x00\r\n") {
		return "", fmt.Errorf("invalid sqlite database path")
	}
	if strings.HasPrefix(clean, "-") {
		clean = "." + string(filepath.Separator) + clean
	}
	return clean, nil
}

func searchConversationsViaSQLiteCLI(ctx context.Context, dbPath, query string, limit int) ([]ConversationSearchResult, error) {
	safeDB, err := safeSQLiteDBArg(dbPath)
	if err != nil {
		return nil, err
	}
	bin, roFlag, err := findSQLiteExecutable()
	if err != nil {
		return nil, err
	}

	runMatch := func(matchExpr string) ([]byte, error) {
		hexMatch := hex.EncodeToString([]byte(matchExpr))
		hexStart := hex.EncodeToString([]byte(ftsMarkerStart))
		hexEnd := hex.EncodeToString([]byte(ftsMarkerEnd))
		sqlQuery := fmt.Sprintf(`SELECT f.conversation_id AS cid, COALESCE(NULLIF(c.title, ''), COALESCE(f.title, '')) AS title, COALESCE(NULLIF(f.project, ''), COALESCE(f.workspace, '')) AS ws, COALESCE(snippet(conversations_fts, 13, CAST(X'%s' AS TEXT), CAST(X'%s' AS TEXT), '...', 12), '') AS snip, COALESCE(f.chunk_first_step, 0) AS step_idx, COALESCE(CAST(c.last_modified_time AS TEXT), '') AS mod_time FROM conversations_fts f LEFT JOIN conversation_summaries c ON f.conversation_id = c.conversation_id WHERE conversations_fts MATCH CAST(X'%s' AS TEXT) ORDER BY rank LIMIT %d`,
			hexStart, hexEnd, hexMatch, limit)

		execCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		args := make([]string, 0, 6)
		if roFlag {
			args = append(args, "-readonly")
		}
		args = append(args, "-cmd", ".timeout 2500", "-json", safeDB, sqlQuery)
		cmd := procutil.HideWindow(exec.CommandContext(execCtx, bin, args...))
		var outBuf, errBuf bytes.Buffer
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf
		if runErr := cmd.Run(); runErr != nil {
			return nil, fmt.Errorf("%w: %s", runErr, strings.TrimSpace(errBuf.String()))
		}
		return bytes.TrimSpace(outBuf.Bytes()), nil
	}

	rawJSON, err := runMatch(query)
	if err != nil {
		// Retry safely as a quoted FTS5 phrase when user input contains bare FTS operators/quotes.
		safePhrase := `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
		rawJSON, err = runMatch(safePhrase)
		if err != nil {
			return nil, err
		}
	}
	if len(rawJSON) == 0 {
		return []ConversationSearchResult{}, nil
	}

	var rows []struct {
		CID     string `json:"cid"`
		Title   string `json:"title"`
		WS      string `json:"ws"`
		Snip    string `json:"snip"`
		StepIdx int    `json:"step_idx"`
		ModTime string `json:"mod_time"`
	}
	if err := json.Unmarshal(rawJSON, &rows); err != nil {
		return nil, err
	}
	out := make([]ConversationSearchResult, 0, len(rows))
	for _, r := range rows {
		title := r.Title
		if title == "" {
			title = "未命名会话"
		}
		cleanSnip, ranges := parseFTSSnippetRanges(r.Snip)
		out = append(out, ConversationSearchResult{
			CascadeID:          r.CID,
			Title:              title,
			WorkspaceName:      r.WS,
			LastModifiedTime:   r.ModTime,
			Snippet:            cleanSnip,
			SnippetMatchRanges: ranges,
			MatchSource:        "conversation_fts",
			MatchedStepIndex:   r.StepIdx,
		})
	}
	return out, nil
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

	dbPath := getAntigravityDBPath()
	if dbPath == "" {
		return nil, fmt.Errorf("antigravity conversation database not found")
	}

	cacheKey := fmt.Sprintf("%s|%d|%s", dbPath, limit, query)
	now := time.Now()
	localStoreMu.RLock()
	if entry, ok := searchCache[cacheKey]; ok && now.Sub(entry.at) < searchCacheTTL {
		cp := append([]ConversationSearchResult(nil), entry.results...)
		localStoreMu.RUnlock()
		return cp, nil
	}
	localStoreMu.RUnlock()

	select {
	case ftsSearchSem <- struct{}{}:
		defer func() { <-ftsSearchSem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	storeCached := func(results []ConversationSearchResult) {
		localStoreMu.Lock()
		if len(searchCache) >= 16 {
			for k, v := range searchCache {
				if time.Since(v.at) >= searchCacheTTL {
					delete(searchCache, k)
				}
			}
			if len(searchCache) >= 16 {
				for k := range searchCache {
					delete(searchCache, k)
					break
				}
			}
		}
		searchCache[cacheKey] = searchCacheEntry{
			results: append([]ConversationSearchResult(nil), results...),
			at:      time.Now(),
		}
		localStoreMu.Unlock()
	}

	// Fast path: lightweight `sqlite3 -readonly -json` subprocess before falling back to python3.
	if cliResults, cliErr := searchConversationsViaSQLiteCLI(ctx, dbPath, query, limit); cliErr == nil {
		storeCached(cliResults)
		return cliResults, nil
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
SELECT f.conversation_id, COALESCE(NULLIF(c.title, ''), f.title), COALESCE(NULLIF(f.project, ''), f.workspace), 
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

	cmd := procutil.HideWindow(exec.CommandContext(execCtx, pyBin, "-c", pyScript, dbPath, query, fmt.Sprintf("%d", limit)))
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

	storeCached(searchResp.Results)
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

		if sqliteRes, sqliteErr := fetchSummariesViaSQLiteCLI(ctx, dbPath); sqliteErr == nil && sqliteRes != nil {
			finalStamp := sqliteDBStamp(dbPath)
			localStoreMu.Lock()
			cachedSummariesMap = sqliteRes
			cachedSummariesAt = time.Now()
			cachedSummariesStamp = finalStamp
			localStoreMu.Unlock()
			seedHierarchyCacheFromSummaries(sqliteRes)
			return sqliteRes, nil
		}

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

		cmd := procutil.HideWindow(exec.CommandContext(execCtx, pyBin, "-c", pyScript, dbPath))
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
		seedHierarchyCacheFromSummaries(res)

		return res, nil
	})
}

func normalizeLocalSummaryStatus(status string) string {
	switch status {
	case "running", "CASCADE_RUN_STATUS_RUNNING":
		return "CASCADE_RUN_STATUS_RUNNING"
	case "error", "CASCADE_RUN_STATUS_ERROR":
		return "CASCADE_RUN_STATUS_ERROR"
	default:
		return "CASCADE_RUN_STATUS_COMPLETED"
	}
}

func seedHierarchyCacheFromSummaries(summaries map[string]map[string]interface{}) {
	if len(summaries) == 0 {
		return
	}
	hierarchyMu.Lock()
	defer hierarchyMu.Unlock()
	for cid, s := range summaries {
		pid, _ := s["parentConversationId"].(string)
		depth := 0
		switch d := s["nestingDepth"].(type) {
		case int:
			depth = d
		case float64:
			depth = int(d)
		}
		hierarchyCache[cid] = hierarchyMeta{parentID: pid, depth: depth}
	}
}

// EnrichSummariesHierarchy populates parentConversationId and nestingDepth on upstream trajectory summaries
// without spawning Python subprocesses on every list poll:
//  1. Promotes embedded trajectoryMetadata.parentConversationId directly in memory.
//  2. Serves immutable parent/depth relationships from hierarchyCache in O(N) memory lookups.
//  3. Queries SQLite only when an unseen conversationId lacks embedded hierarchy metadata.
func EnrichSummariesHierarchy(ctx context.Context, summaries map[string]map[string]interface{}) {
	if len(summaries) == 0 {
		return
	}

	var unknownIDs []string
	hierarchyMu.RLock()
	for id, s := range summaries {
		if pid, ok := s["parentConversationId"].(string); ok && pid != "" {
			continue
		}
		if meta, ok := s["trajectoryMetadata"].(map[string]interface{}); ok {
			if pid, ok := meta["parentConversationId"].(string); ok && strings.TrimSpace(pid) != "" {
				s["parentConversationId"] = strings.TrimSpace(pid)
				continue
			}
		}
		if h, ok := hierarchyCache[id]; ok {
			if h.parentID != "" {
				s["parentConversationId"] = h.parentID
			}
			if h.depth > 0 {
				s["nestingDepth"] = h.depth
			}
			continue
		}
		unknownIDs = append(unknownIDs, id)
	}
	hierarchyMu.RUnlock()

	if len(unknownIDs) == 0 {
		return
	}

	dbPath := getAntigravityDBPath()
	if dbPath == "" {
		// Cache negative lookup in-memory when no local DB exists (e.g., tests or older Antigravity)
		hierarchyMu.Lock()
		for _, id := range unknownIDs {
			if _, exists := hierarchyCache[id]; !exists {
				hierarchyCache[id] = hierarchyMeta{}
			}
		}
		hierarchyMu.Unlock()
		return
	}

	if rows, err := fetchHierarchyViaSQLiteCLI(ctx, dbPath); err == nil {
		found := make(map[string]hierarchyMeta, len(rows))
		for _, r := range rows {
			found[r.ConversationID] = hierarchyMeta{
				parentID: r.ParentConversationID,
				depth:    r.NestingDepth,
			}
		}
		hierarchyMu.Lock()
		for cid, meta := range found {
			hierarchyCache[cid] = meta
		}
		for _, id := range unknownIDs {
			if _, ok := found[id]; !ok {
				// Mark known root session so we don't re-query SQLite on every poll before DB flushes it
				hierarchyCache[id] = hierarchyMeta{}
			}
		}
		hierarchyMu.Unlock()

		for _, id := range unknownIDs {
			if meta, ok := found[id]; ok {
				if s := summaries[id]; s != nil {
					if meta.parentID != "" {
						s["parentConversationId"] = meta.parentID
					}
					if meta.depth > 0 {
						s["nestingDepth"] = meta.depth
					}
				}
			}
		}
		return
	}

	// Final fallback if sqlite3 CLI is unavailable
	if localSummaries, err := FetchLocalConversationSummaries(ctx); err == nil {
		hierarchyMu.Lock()
		for _, id := range unknownIDs {
			if loc, ok := localSummaries[id]; ok {
				pid, _ := loc["parentConversationId"].(string)
				depth, _ := loc["nestingDepth"].(int)
				hierarchyCache[id] = hierarchyMeta{parentID: pid, depth: depth}
				if s := summaries[id]; s != nil {
					if pid != "" {
						s["parentConversationId"] = pid
					}
					if depth > 0 {
						s["nestingDepth"] = depth
					}
				}
			} else {
				hierarchyCache[id] = hierarchyMeta{}
			}
		}
		hierarchyMu.Unlock()
	}
}

type sqliteHierarchyRow struct {
	ConversationID       string `json:"conversation_id"`
	ParentConversationID string `json:"parent_conversation_id"`
	NestingDepth         int    `json:"nesting_depth"`
}

func fetchHierarchyViaSQLiteCLI(ctx context.Context, dbPath string) ([]sqliteHierarchyRow, error) {
	safeDB, err := safeSQLiteDBArg(dbPath)
	if err != nil {
		return nil, err
	}
	bin, roFlag, err := findSQLiteExecutable()
	if err != nil {
		return nil, err
	}
	execCtx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()

	const sqlQuery = `SELECT conversation_id, COALESCE(parent_conversation_id, '') AS parent_conversation_id, COALESCE(nesting_depth, 0) AS nesting_depth FROM conversation_summaries`
	args := make([]string, 0, 6)
	if roFlag {
		args = append(args, "-readonly")
	}
	args = append(args, "-cmd", ".timeout 2500", "-json", safeDB, sqlQuery)

	cmd := procutil.HideWindow(exec.CommandContext(execCtx, bin, args...))
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("sqlite3 hierarchy query error: %w (%s)", err, errBuf.String())
	}
	trimmed := bytes.TrimSpace(outBuf.Bytes())
	if len(trimmed) == 0 {
		return nil, nil
	}
	var rows []sqliteHierarchyRow
	if err := json.Unmarshal(trimmed, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func fetchSummariesViaSQLiteCLI(ctx context.Context, dbPath string) (map[string]map[string]interface{}, error) {
	safeDB, err := safeSQLiteDBArg(dbPath)
	if err != nil {
		return nil, err
	}
	bin, roFlag, err := findSQLiteExecutable()
	if err != nil {
		return nil, err
	}
	execCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	const sqlQuery = `SELECT conversation_id, COALESCE(title, '') AS title, COALESCE(preview, '') AS preview, COALESCE(step_count, 0) AS step_count, COALESCE(CAST(last_modified_time AS TEXT), '') AS last_modified_time, COALESCE(workspace_uris, '') AS workspace_uris, COALESCE(status, '') AS status, COALESCE(parent_conversation_id, '') AS parent_conversation_id, COALESCE(nesting_depth, 0) AS nesting_depth FROM conversation_summaries ORDER BY last_modified_time DESC`
	args := make([]string, 0, 6)
	if roFlag {
		args = append(args, "-readonly")
	}
	args = append(args, "-cmd", ".timeout 2500", "-json", safeDB, sqlQuery)

	cmd := procutil.HideWindow(exec.CommandContext(execCtx, bin, args...))
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("sqlite3 summaries query error: %w (%s)", err, errBuf.String())
	}

	res := make(map[string]map[string]interface{})
	trimmed := bytes.TrimSpace(outBuf.Bytes())
	if len(trimmed) == 0 {
		return res, nil
	}

	var rows []struct {
		ConversationID       string `json:"conversation_id"`
		Title                string `json:"title"`
		Preview              string `json:"preview"`
		StepCount            int    `json:"step_count"`
		LastModifiedTime     string `json:"last_modified_time"`
		WorkspaceURIs        string `json:"workspace_uris"`
		Status               string `json:"status"`
		ParentConversationID string `json:"parent_conversation_id"`
		NestingDepth         int    `json:"nesting_depth"`
	}
	if err := json.Unmarshal(trimmed, &rows); err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.ConversationID == "" {
			continue
		}
		title := r.Title
		if title == "" {
			title = "未命名会话"
		}
		res[r.ConversationID] = map[string]interface{}{
			"cascadeId":            r.ConversationID,
			"status":               normalizeLocalSummaryStatus(r.Status),
			"lastModifiedTime":     r.LastModifiedTime,
			"stepCount":            r.StepCount,
			"summary":              r.Preview,
			"parentConversationId": r.ParentConversationID,
			"nestingDepth":         r.NestingDepth,
			"trajectory": map[string]interface{}{
				"cascadeId": r.ConversationID,
				"summary":   r.Preview,
				"annotations": map[string]interface{}{
					"title": title,
				},
			},
		}
	}
	return res, nil
}

var (
	sqliteBinOnce     sync.Once
	cachedSQLiteBin   string
	cachedSQLiteRO    bool
	cachedSQLiteErr   error
)

func findSQLiteExecutable() (string, bool, error) {
	sqliteBinOnce.Do(func() {
		path, err := exec.LookPath("sqlite3")
		if err != nil || !isExecutableValid(path) {
			cachedSQLiteErr = fmt.Errorf("sqlite3 not found in PATH")
			return
		}
		if out, err := procutil.HideWindow(exec.Command(path, "-readonly", "-json", ":memory:", "SELECT 1 AS ok")).Output(); err == nil && bytes.Contains(out, []byte(`"ok"`)) {
			cachedSQLiteBin = path
			cachedSQLiteRO = true
			return
		}
		if out, err := procutil.HideWindow(exec.Command(path, "-json", ":memory:", "SELECT 1 AS ok")).Output(); err == nil && bytes.Contains(out, []byte(`"ok"`)) {
			cachedSQLiteBin = path
			cachedSQLiteRO = false
			return
		}
		cachedSQLiteErr = fmt.Errorf("sqlite3 does not support -json output")
	})
	return cachedSQLiteBin, cachedSQLiteRO, cachedSQLiteErr
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
				cmd := procutil.HideWindow(exec.Command(bin, "-c", "import sys; sys.exit(0)"))
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
