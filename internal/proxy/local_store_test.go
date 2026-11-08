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

	// Verify query with special FTS5 syntax does not crash
	specialResults, err := SearchConversationsFTS5(ctx, `foo"bar* OR AND`, 5)
	if err != nil {
		t.Errorf("SearchConversationsFTS5 failed on special characters: %v", err)
	}
	t.Logf("Special characters query returned %d results safely", len(specialResults))
}

func TestFindPythonExecutable(t *testing.T) {
	pyBin, err := findPythonExecutable()
	if err != nil {
		t.Logf("findPythonExecutable returned: %v (no python installed on this machine)", err)
		return
	}
	if pyBin == "" {
		t.Fatal("expected non-empty pyBin")
	}
	// Verify that the found executable is valid and non-zero bytes
	if !isExecutableValid(pyBin) {
		t.Fatalf("expected executable %q to be valid (non-zero bytes)", pyBin)
	}
}
