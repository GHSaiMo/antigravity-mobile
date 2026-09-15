# Security & Performance Audit — antigravity-mobile

> Audit date: 2026-09-15 · Reviewer: Antigravity
> ✅ P1 fixed 2026-09-15 — trajCacheMu upgraded to sync.RWMutex; all read-only paths use RLock/RUnlock
> ✅ P2 fixed 2026-09-15 — ParseTrajectoryDetails moved outside trajCacheMu; data pointer copied under short RLock
> ✅ P9 fixed 2026-09-15 — HasSyncedHistoricalTrajectories uses RLock; loadedCascadesMu upgraded to sync.RWMutex
> ✅ S1 fixed 2026-09-15 — RateLimiter: added gc() goroutine (2min interval) that evicts fully-expired keys
> ✅ S2 fixed 2026-09-15 — Admin token moved from os.Setenv/os.Getenv to atomic.Value (SetAdminToken/GetAdminToken)
> ✅ S3 fixed 2026-09-15 — os.RemoveAll moved into success branch only; tombstone set pre-emptively, cleared on 4xx
> ✅ S4 fixed 2026-09-15 — CSP script-src 'unsafe-inline' replaced with SHA-256 hashes of app.js, mermaid.min.js, sw.js
> ✅ S5 fixed 2026-09-15 — FRP_SERVER_ADDR removed from WebSocket origin whitelist; comment explains CSWSH risk
> ✅ S6 fixed 2026-09-15 — Added writeJSONError() helper; all fmt.Sprintf error injections replaced
> ✅ S7 fixed 2026-09-15 — loadOrCreateAuthSalt returns "" on rand.Read failure instead of hardcoded fallback
> ✅ S8 fixed 2026-09-15 — ValidateToken transparently upgrades legacy unsalted tokens to salted hash on first use
> ✅ P3 fixed 2026-09-15 — bufferedResponseWriter now uses GetLargeBuffer()/PutLargeBuffer() pool; defer release() in all callers
> ✅ P4 fixed 2026-09-15 — GetAllCascadeTrajectories uses context deadline on mediumClient instead of per-request http.Client
> ✅ P5 fixed 2026-09-15 — UpdateLastSeen: per-request goroutine replaced with buffered channel worker (EnqueueLastSeen)
> ✅ P6 fixed 2026-09-15 — Content-Length checked before reading body in handleSendUserCascadeMessage to fast-reject payloads exceeding 50MB
> ✅ P7 fixed 2026-09-15 — Stream first push is now immediate (synchronous fetchAndSend before select loop); 250ms ticker thereafter
> ✅ P8 fixed 2026-09-15 — Combined 4 sequential image URL regular expressions into a single composite regex with alternation, reducing 4 text passes to 1
> ✅ S9 fixed 2026-09-15 — WSTicketStore short-lived one-time ticket system implemented; client exchanges device token via POST /api/v1/auth/ws-ticket and connects with ?ticket=...; device token no longer exposed in WS query strings

---

## Executive Summary

The codebase is well-structured overall. Auth plumbing (constant-time comparison, salted token hashing, one-time pairing, loopback-trust with tunnel awareness) is solid. The main concerns are: one medium-severity memory leak in the rate limiter, a few places where CPU work happens under a mutex, pooled buffers that are available but not wired up to all the code paths that need them, and some minor security hygiene issues.

---

## 🔴 Security Findings

### S1 · RateLimiter — Unbounded Memory Growth (Medium)
**File**: [`ratelimit.go`](file:///Users/hal9000/Projects/antigravity-mobile/internal/auth/ratelimit.go#L8-L42)

`RateLimiter.hits` is a `map[string][]time.Time` that is **never evicted for stale keys**. Inline cleanup only prunes old timestamps for the *same key* during `Allow()`. An attacker rotating IPs or crafting thousands of unique `pair:` / `session:` keys will grow the map without bound.

```go
// Current: stale keys accumulate forever
q := l.hits[key]
kept := q[:0]
for _, ts := range q { ... } // only cleans THIS key's old timestamps
```

**Fix**: Add a periodic GC goroutine (or max-key LRU cap):
```go
func (l *RateLimiter) startGC(interval time.Duration) {
    go func() {
        ticker := time.NewTicker(interval)
        defer ticker.Stop()
        for range ticker.C {
            l.mu.Lock()
            cutoff := time.Now().Add(-maxWindow)
            for k, ts := range l.hits {
                if len(ts) == 0 || ts[len(ts)-1].Before(cutoff) {
                    delete(l.hits, k)
                }
            }
            l.mu.Unlock()
        }
    }()
}
```

---

### S2 · Admin Token Stored in Process Environment (Low-Medium)
**File**: [`admin_token.go`](file:///Users/hal9000/Projects/antigravity-mobile/internal/auth/admin_token.go#L30-L52), [`handler.go`](file:///Users/hal9000/Projects/antigravity-mobile/internal/auth/handler.go#L355)

`EnsureAdminToken` loads the token from disk and puts it into `os.Setenv("ADMIN_TOKEN", tok)`. `isAuthorizedAdmin` then re-reads it with `os.Getenv("ADMIN_TOKEN")` on **every admin request**. Two problems:

1. Environment variables are visible to all goroutines, child processes, and system-level tools (e.g., `/proc/PID/environ` on Linux).
2. `os.Setenv` / `os.Getenv` take a global mutex — unnecessary per-request contention.

**Fix**: Store the token in a package-level `atomic.Value` or `sync.Once`-cached string inside `auth` package, and read it from there instead of the environment.

```go
var cachedAdminToken atomic.Value

func SetAdminToken(tok string) { cachedAdminToken.Store(tok) }
func GetAdminToken() string {
    if v := cachedAdminToken.Load(); v != nil { return v.(string) }
    return ""
}
```

---

### S3 · `handleDeleteCascadeTrajectory` — Files Deleted Before Upstream Confirmation (Low-Medium)
**File**: [`proxy.go`](file:///Users/hal9000/Projects/antigravity-mobile/internal/proxy/proxy.go#L884-L892)

`os.RemoveAll(brainDir)` is called **before** `rp.ServeHTTP(rec, fwdReq)` returns. If upstream rejects the deletion (e.g., cascade is mid-run), local brain files are already gone and unrecoverable.

```go
// BUG: files deleted before upstream confirms
_ = os.RemoveAll(brainDir)   // line 890

rec := newBufferedResponseWriter()
rp.ServeHTTP(rec, fwdReq)   // upstream may return 4xx!
```

**Fix**: Move the `os.RemoveAll` call to the success branch (lines 901–918) where it already also appears — the pre-emptive deletion on lines 888–891 should be removed entirely.

---

### S4 · CSP Allows `'unsafe-inline'` on `script-src` (Low)
**File**: [`middleware.go`](file:///Users/hal9000/Projects/antigravity-mobile/internal/auth/middleware.go#L195)

```go
"script-src 'self' 'unsafe-inline'"
```

`'unsafe-inline'` eliminates most of CSP's XSS protection. For a gateway reachable over a public relay or DDNS, any XSS in the web UI escapes into authenticated session context.

**Fix**: Replace with a nonce-based or hash-based `script-src`. The current web/app.js is a static file, so a precomputed hash works:
```
script-src 'self' 'sha256-<hash-of-app.js>'
```

---

### S5 · `IsAllowedOrigin` Trusts `FRP_SERVER_ADDR` as Browser Origin (Low)
**File**: [`ws.go`](file:///Users/hal9000/Projects/antigravity-mobile/internal/proxy/ws.go#L65-L67)

```go
if fa := strings.TrimSpace(os.Getenv("FRP_SERVER_ADDR")); fa != "" {
    trustedHosts = append(trustedHosts, strings.ToLower(fa))
}
```

`FRP_SERVER_ADDR` is the tunnel server's address. Any web page hosted on that domain (e.g., a shared FRP host) can now send a WebSocket upgrade with `Origin: http://frp.example.com` and pass the check — a Cross-Site WebSocket Hijacking vector. The FRP server address is a network endpoint, not a browser origin.

**Fix**: Remove `FRP_SERVER_ADDR` from the trusted-origin list. Users who need to allow it can add it to `ALLOWED_ORIGINS` explicitly.

---

### S6 · Error Messages Reflected Unescaped into JSON Responses (Low)
**File**: [`proxy.go`](file:///Users/hal9000/Projects/antigravity-mobile/internal/proxy/proxy.go#L1051), multiple locations

```go
http.Error(w, fmt.Sprintf(`{"error":"invalid request: %s"}`, err.Error()), ...)
```

If `err.Error()` contains `"` or `\`, the JSON is malformed. If it contains attacker-controlled content, it's an injection point. Affects `HandleCascadeInteraction`, `handleCascadeTaskStop`, `HandleCascadeInteraction`, etc.

**Fix**: Use `json.Marshal` for all error structs:
```go
w.Header().Set("Content-Type", "application/json")
w.WriteHeader(http.StatusBadRequest)
json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
```

---

### S7 · `auth_salt` Falls Back to Hardcoded Bytes on `rand.Read` Failure (Low)
**File**: [`store.go`](file:///Users/hal9000/Projects/antigravity-mobile/internal/auth/store.go#L69)

```go
if _, err := rand.Read(raw); err != nil {
    return hex.EncodeToString([]byte("antigravity-mobile-fallback-salt"))
}
```

If `rand.Read` fails (rare but possible under kernel entropy exhaustion), all installations share the same salt, making token hashes trivially rainbow-table-able across the entire user base.

**Fix**: Return an error and let the caller surface a fatal startup failure instead of silently degrading.

---

### S8 · Legacy Unsalted SHA-256 Tokens Never Rotated (Low)
**File**: [`store.go`](file:///Users/hal9000/Projects/antigravity-mobile/internal/auth/store.go#L220-L224)

The `LegacyHashToken` fallback in `ValidateToken` keeps backward compatibility but provides weaker hash strength (no salt = rainbow-table vulnerable). There is no migration trigger to upgrade legacy tokens.

**Fix**: On successful legacy-hash validation, immediately re-hash and persist the device with the modern salted hash, then clear the legacy entry. This is a transparent in-place migration.

---

### S9 · Auth Token Visible in Query String for WebSocket / Raw File Routes (Informational)
**File**: [`middleware.go`](file:///Users/hal9000/Projects/antigravity-mobile/internal/auth/middleware.go#L44-L55)

`?auth_token=...` will appear in: server access logs, browser history, `Referer` headers from redirects, CDN/proxy logs, and network captures. This is a documented tradeoff for WebSocket + native apps, but worth noting that the `Secure` flag on the cookie partially mitigates this for browser clients.

---

## 🟡 Performance Findings

### P1 · `trajCacheMu` is `sync.Mutex` — Should be `sync.RWMutex` (High Impact)
**File**: [`trajectory.go`](file:///Users/hal9000/Projects/antigravity-mobile/internal/proxy/trajectory.go#L115)

```go
trajCacheMu sync.Mutex  // read >> write, but exclusive lock used everywhere
```

The trajectory cache is read on every 250ms stream tick per connected client, and on every `GetAllCascadeTrajectories` call, but written only on cache miss or invalidation. Using a plain `sync.Mutex` means concurrent readers block each other.

**Fix**: Change to `sync.RWMutex` and use `RLock/RUnlock` for reads, `Lock/Unlock` only for writes.

---

### P2 · CPU Work Under `trajCacheMu` Lock (High Impact)
**File**: [`proxy.go`](file:///Users/hal9000/Projects/antigravity-mobile/internal/proxy/proxy.go#L1471-L1491)

```go
defaultTrajCache.trajCacheMu.Lock()
cached, ok := defaultTrajCache.trajCache[cid]
if ok && cached != nil && cached.data != nil {
    details := p.ParseTrajectoryDetails(cached.data)  // ← JSON parsing under lock!
```

`ParseTrajectoryDetails` does significant JSON traversal while holding the global trajectory cache mutex. Any concurrent stream client or cache-write blocks for the duration of this parsing.

**Fix**: Copy the pointer under the lock, release immediately, then parse:
```go
defaultTrajCache.trajCacheMu.Lock()
cached, ok := defaultTrajCache.trajCache[cid]
var dataCopy *upstreamTrajectoryResp
if ok && cached != nil {
    dataCopy = cached.data
}
defaultTrajCache.trajCacheMu.Unlock()

if dataCopy != nil {
    details := p.ParseTrajectoryDetails(dataCopy)
    ...
}
```

---

### P3 · `bufferedResponseWriter` Doesn't Use the Available Buffer Pool (Medium Impact)
**File**: [`proxy.go`](file:///Users/hal9000/Projects/antigravity-mobile/internal/proxy/proxy.go#L511-L516), [`pool.go`](file:///Users/hal9000/Projects/antigravity-mobile/internal/proxy/pool.go)

```go
func newBufferedResponseWriter() *bufferedResponseWriter {
    return &bufferedResponseWriter{
        header:     make(http.Header),
        statusCode: http.StatusOK,
    }
}
// body bytes.Buffer — always a fresh heap allocation!
```

`pool.go` exports `GetLargeBuffer()` / `PutLargeBuffer()` precisely for this use case, but `bufferedResponseWriter` allocates a new `bytes.Buffer` on every call. This hits every: `handleSendUserCascadeMessage`, `handleDeleteCascadeTrajectory`, `handleDeleteAgentMessage`, `handleUpdateConversationAnnotations`.

**Fix**:
```go
type bufferedResponseWriter struct {
    header     http.Header
    body       *bytes.Buffer  // pooled
    statusCode int
}

func newBufferedResponseWriter() *bufferedResponseWriter {
    return &bufferedResponseWriter{
        header:     make(http.Header),
        body:       GetLargeBuffer(),
        statusCode: http.StatusOK,
    }
}

func (b *bufferedResponseWriter) release() {
    PutLargeBuffer(b.body)
    b.body = nil
}
```

Call `defer rw.release()` after each handler finishes writing the response.

---

### P4 · New `http.Client` Allocated Per `GetAllCascadeTrajectories` Request (Medium Impact)
**File**: [`proxy.go`](file:///Users/hal9000/Projects/antigravity-mobile/internal/proxy/proxy.go#L1264-L1268)

```go
client := &http.Client{
    Timeout:   4 * time.Second,
    Transport: p.transport,
}
```

This allocates a new client struct on every list-trajectories call. While `http.Client` is lightweight, the pre-built tiered clients (`shortClient`, `mediumClient`, `longClient`) were specifically added to avoid this. A 4s client is not among them.

**Fix**: Add `listClient *http.Client` (4s timeout) to `Proxy` and initialize in `NewProxy`, or derive a context deadline from the existing `mediumClient`:
```go
ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
defer cancel()
req, _ = http.NewRequestWithContext(ctx, ...)
resp, err = p.mediumClient.Do(req)
```

---

### P5 · `UpdateLastSeen` Spawns a Goroutine per Authenticated Request (Medium Impact)
**File**: [`middleware.go`](file:///Users/hal9000/Projects/antigravity-mobile/internal/auth/middleware.go#L181)

```go
go store.UpdateLastSeen(device.DeviceID, r.RemoteAddr)
```

Each authenticated HTTP request spawns a goroutine. The debounce logic is good, but goroutine creation + scheduling overhead accumulates on high-frequency WebSocket or stream polling. The goroutines often do zero work (the 30s fast-path check).

**Fix**: Use a bounded channel-based worker:
```go
type updateMsg struct{ deviceID, ip string }
var updateCh = make(chan updateMsg, 64)

func init() {
    go func() {
        for m := range updateCh {
            store.UpdateLastSeen(m.deviceID, m.ip)
        }
    }()
}

// In middleware, replace `go store.UpdateLastSeen(...)` with:
select {
case updateCh <- updateMsg{device.DeviceID, r.RemoteAddr}:
default: // drop if channel full — debounce handles correctness
}
```

---

### P6 · `handleSendUserCascadeMessage` Reads Full 50MB Body Before Content-Hash Dedup Check (Low Impact)
**File**: [`proxy.go`](file:///Users/hal9000/Projects/antigravity-mobile/internal/proxy/proxy.go#L611-L664)

The `clientMsgID` check (cheap) happens first (good), but the content-hash dedup path always reads up to 50MB before computing the hash. For non-idempotent large image payloads, this is necessary — but the `Content-Length` header could be used to fast-reject requests that are trivially too large before reading.

---

### P7 · `stream.go` Initial Poll Tick at 50ms Instead of Immediate (Low Impact)
**File**: [`stream.go`](file:///Users/hal9000/Projects/antigravity-mobile/internal/proxy/stream.go#L160-L167)

```go
ticker := time.NewTicker(50 * time.Millisecond) // immediate first check
```

The comment says "immediate" but `time.NewTicker(50ms)` is still a 50ms delay before the first fetch. The intent is the `touchCh` path (instant wake on external touch). The initial fetch should be truly immediate.

**Fix**: Use a boolean or `firstPush` flag to trigger a fetch synchronously before the loop, eliminating the 50ms latency on first connect:
```go
// Before the select loop:
rawResp, err := p.fetchUpstreamTrajectoryWithMaxAge(cascadeID, port, token, 0)
// ... process and send firstPush
ticker := time.NewTicker(250 * time.Millisecond)
```

---

### P8 · `extractImageURLsFromText` Runs 4 Regex Scans Sequentially (Low Impact)
**File**: [`trajectory.go`](file:///Users/hal9000/Projects/antigravity-mobile/internal/proxy/trajectory.go#L177-L189)

Four separate regexes are applied sequentially to (potentially large) trajectory text strings. For long agent outputs, this is 4× the linear scan work.

**Fix**: Combine into a single composite regex with named groups, or switch to a single-pass `strings.Builder`-based parser for the most common patterns.

---

### P9 · `HasSyncedHistoricalTrajectories` Uses `sync.Mutex` for a Read-Only Check (Low Impact)
**File**: [`trajectory.go`](file:///Users/hal9000/Projects/antigravity-mobile/internal/proxy/trajectory.go#L244-L248)

```go
func HasSyncedHistoricalTrajectories(port int) bool {
    defaultTrajCache.loadedCascadesMu.Lock()     // write lock for a read!
    defer defaultTrajCache.loadedCascadesMu.Unlock()
    return port > 0 && port == defaultTrajCache.lastSyncedPort
}
```

`loadedCascadesMu` is `sync.Mutex` (not RWMutex), and this read function acquires an exclusive write lock. This blocks any concurrent writer that needs to update `loadedCascades`.

**Fix**: Change `loadedCascadesMu` to `sync.RWMutex` and use `RLock/RUnlock` in `HasSyncedHistoricalTrajectories`.

---

## Summary Table

| ID  | Severity | Category    | File                | Description | Status |
|-----|----------|-------------|---------------------|-------------|--------|
| S1  | 🔴 Medium | Security    | `ratelimit.go`      | RateLimiter key map grows without bound | ✅ Fixed |
| S2  | 🟠 Low-Med | Security   | `admin_token.go`    | Admin token stored in process environment | ✅ Fixed |
| S3  | 🟠 Low-Med | Security   | `proxy.go`          | `os.RemoveAll` before upstream confirms delete | ✅ Fixed |
| S4  | 🟡 Low    | Security    | `middleware.go`     | CSP `script-src 'unsafe-inline'` | ✅ Fixed |
| S5  | 🟡 Low    | Security    | `ws.go`             | FRP_SERVER_ADDR trusted as browser origin | ✅ Fixed |
| S6  | 🟡 Low    | Security    | `proxy.go`          | Error strings unescaped into JSON | ✅ Fixed |
| S7  | 🟡 Low    | Security    | `store.go`          | Hardcoded fallback salt on rand failure | ✅ Fixed |
| S8  | 🟡 Low    | Security    | `store.go`          | Legacy unsalted tokens never rotated | ✅ Fixed |
| S9  | ℹ️ Info   | Security    | `middleware.go`+`app.js` | Token in query string for WS endpoints | ✅ Fixed |
| P1  | 🔴 High   | Performance | `trajectory.go`     | `trajCacheMu` should be RWMutex | ✅ Fixed |
| P2  | 🔴 High   | Performance | `proxy.go`          | JSON parsing under global cache lock | ✅ Fixed |
| P3  | 🟠 Medium | Performance | `proxy.go`+`pool.go`| Buffered response writer ignores buffer pool | ✅ Fixed |
| P4  | 🟠 Medium | Performance | `proxy.go`          | New `http.Client` per trajectory list request | ✅ Fixed |
| P5  | 🟠 Medium | Performance | `middleware.go`     | Goroutine spawn per authenticated request | ✅ Fixed |
| P6  | 🟡 Low    | Performance | `proxy.go`          | 50MB body read before content-hash dedup | ✅ Fixed |
| P7  | 🟡 Low    | Performance | `stream.go`         | 50ms delay on first stream push | ✅ Fixed |
| P8  | 🟡 Low    | Performance | `trajectory.go`     | 4 sequential regex scans on same text | ✅ Fixed |
| P9  | 🟡 Low    | Performance | `trajectory.go`     | Write lock used for read-only sync check | ✅ Fixed |

---

## Recommended Priority Order

1. **P1 + P2** (one PR) — Upgrade `trajCacheMu` to RWMutex and remove CPU work from inside it. This is the highest-leverage performance fix.
2. **S1** — Add RateLimiter GC to prevent OOM under IP-rotation attacks.
3. **S3** — Move `os.RemoveAll` to the success-only branch.
4. **P3** — Wire `GetLargeBuffer()` into `bufferedResponseWriter`.
5. **S2** — Move admin token out of `os.Setenv`/`os.Getenv`.
6. **S5** — Remove `FRP_SERVER_ADDR` from WebSocket origin whitelist.
7. **S8** — Add in-place legacy token upgrade on successful validation.
8. **P4 + P5** — Clean up per-request client allocation and goroutine spawn.
