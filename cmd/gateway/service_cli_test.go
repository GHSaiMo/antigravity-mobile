package main

import (
	"testing"
)

func TestHasHelpArg(t *testing.T) {
	if !hasHelpArg([]string{"-h"}) || !hasHelpArg([]string{"--help"}) || !hasHelpArg([]string{"HELP"}) {
		t.Fatalf("expected help args to be recognized")
	}
	if hasHelpArg([]string{"-n", "50"}) {
		t.Fatalf("did not expect -n 50 to be treated as help")
	}
}

func TestNormalizeTopLevelCommandService(t *testing.T) {
	if got := normalizeTopLevelCommand("service"); got != "service" {
		t.Fatalf("normalizeTopLevelCommand(service) = %q, want service", got)
	}
}
