package proxy

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHandleCascadeChapters_AsyncPollConvergence(t *testing.T) {
	origInterval := chapterPollInterval
	origMaxWait := chapterPollMaxWait
	chapterPollInterval = 15 * time.Millisecond
	chapterPollMaxWait = 200 * time.Millisecond
	t.Cleanup(func() {
		chapterPollInterval = origInterval
		chapterPollMaxWait = origMaxWait
	})

	var calls int32
	p := newFakeLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/GetChapterTitles") {
			http.NotFound(w, r)
			return
		}
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			// First call triggers async generation and returns only the first cached chapter
			// (also omits startStepIndex: 0 to simulate proto3 JSON emission)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"chapters": []map[string]interface{}{
					{
						"endStepIndexExclusive": 120,
						"title":                 "Initial codebase exploration",
						"generatedAt":           "2026-11-10T04:59:24Z",
					},
				},
			})
			return
		}
		// Second call returns both chapters
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"chapters": []map[string]interface{}{
				{
					"endStepIndexExclusive": 120,
					"title":                 "Initial codebase exploration",
					"generatedAt":           "2026-11-10T04:59:24Z",
				},
				{
					"startStepIndex":        120,
					"endStepIndexExclusive": 240,
					"title":                 "Implementing Antigravity 2.30.0 chapter TOC",
					"generatedAt":           "2026-11-10T04:59:25Z",
				},
			},
		})
	})

	body := `{"cascadeId":"casc-chap-1","ranges":[{"startStepIndex":0,"endStepIndexExclusive":120},{"startStepIndex":120,"endStepIndexExclusive":240}]}`
	w := postJSON(t, p.HandleCascadeChapters, "/gateway/cascade/chapters", body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp CascadeChaptersResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Chapters) != 2 {
		t.Fatalf("expected 2 chapters after poll convergence, got %+v", resp.Chapters)
	}
	if resp.Chapters[0].StartStepIndex != 0 || resp.Chapters[0].EndStepIndexExclusive != 120 || resp.Chapters[0].Title != "Initial codebase exploration" {
		t.Fatalf("unexpected chapter[0]: %+v", resp.Chapters[0])
	}
	if resp.Chapters[1].StartStepIndex != 120 || resp.Chapters[1].EndStepIndexExclusive != 240 || resp.Chapters[1].Title != "Implementing Antigravity 2.30.0 chapter TOC" {
		t.Fatalf("unexpected chapter[1]: %+v", resp.Chapters[1])
	}
	if atomic.LoadInt32(&calls) < 2 {
		t.Fatalf("expected at least 2 upstream calls, got %d", atomic.LoadInt32(&calls))
	}
}
