package notifier

import (
	"testing"
	"time"

	"antigravity-mobile/internal/proxy"
)

func TestDismissedProceedSuppressesApprovalKeyAndLiveActivityState(t *testing.T) {
	details := &proxy.TrajectoryDetails{
		CascadeID:        "44444444-4444-4444-4444-444444444444",
		Title:            "Supply Strategy Procurement",
		Status:           "CASCADE_RUN_STATUS_IDLE",
		TotalSteps:       122,
		CanProceed:       true,
		ProceedDismissed: false,
	}

	if got := approvalKeyForDetails(details); got != "proceed:122" {
		t.Fatalf("expected active proceed approval key 'proceed:122', got %q", got)
	}
	stateActive := BuildContentStateForLang(details, time.Now(), "en")
	if !stateActive.HasPendingAction || stateActive.Status != "WAITING_APPROVAL" {
		t.Fatalf("expected WAITING_APPROVAL when proceed not dismissed, got status=%s hasPending=%v", stateActive.Status, stateActive.HasPendingAction)
	}

	details.ProceedDismissed = true
	if got := approvalKeyForDetails(details); got != "" {
		t.Fatalf("expected dismissed proceed approval key to be empty, got %q", got)
	}
	stateDismissed := BuildContentStateForLang(details, time.Now(), "en")
	if stateDismissed.HasPendingAction || stateDismissed.Status != "COMPLETED" {
		t.Fatalf("expected COMPLETED without pending action once proceed dismissed, got status=%s hasPending=%v", stateDismissed.Status, stateDismissed.HasPendingAction)
	}
	redactedDismissed := BuildRedactedContentStateForLang(details, time.Now(), "en")
	if redactedDismissed.HasPendingAction || redactedDismissed.Status != "COMPLETED" {
		t.Fatalf("expected redacted state COMPLETED without pending action once proceed dismissed, got status=%s hasPending=%v", redactedDismissed.Status, redactedDismissed.HasPendingAction)
	}
}
