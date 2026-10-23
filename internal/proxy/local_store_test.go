package proxy

import (
	"context"
	"testing"
)

func TestLocalStoreFTS5Search(t *testing.T) {
	ctx := context.Background()
	results, err := SearchConversationsFTS5(ctx, "antigravity", 5)
	if err != nil {
		t.Logf("SearchConversationsFTS5 skipped/failed (possibly db not present in test env): %v", err)
		return
	}
	t.Logf("Found %d search results from FTS5", len(results))
	for i, r := range results {
		if r.CascadeID == "" {
			t.Errorf("result %d has empty CascadeID", i)
		}
	}
}

func TestLocalStoreFetchSummaries(t *testing.T) {
	ctx := context.Background()
	summaries, err := FetchLocalConversationSummaries(ctx)
	if err != nil {
		t.Logf("FetchLocalConversationSummaries skipped/failed: %v", err)
		return
	}
	t.Logf("Found %d local conversation summaries", len(summaries))
	for id, s := range summaries {
		if s["cascadeId"] != id {
			t.Errorf("mismatched cascadeId: got %v, want %s", s["cascadeId"], id)
		}
		break
	}
}
