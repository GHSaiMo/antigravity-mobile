package proxy

import (
	"encoding/json"
	"testing"
	"time"

	"antigravity-mobile/internal/inspector"
)

func TestParseTrajectoryDetails_CanProceed(t *testing.T) {
	rawJSON := `{
		"status": "CASCADE_RUN_STATUS_IDLE",
		"trajectory": {
			"cascadeId": "test-cascade-123",
			"steps": [
				{
					"type": "CORTEX_STEP_TYPE_USER_INPUT",
					"status": "CORTEX_STEP_STATUS_DONE",
					"userInput": {
						"userResponse": "Please generate a plan"
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
					"status": "CORTEX_STEP_STATUS_DONE",
					"plannerResponse": {
						"response": "Here is the implementation plan"
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_CODE_ACTION",
					"status": "CORTEX_STEP_STATUS_DONE",
					"codeAction": {
						"isArtifactFile": true,
						"artifactMetadata": {
							"summary": "Test plan",
							"requestFeedback": true,
							"userFacing": true
						},
						"actionResult": {
							"edit": {
								"absoluteUri": "file:///path/to/implementation_plan.md",
								"createFile": true
							}
						}
					}
				}
			]
		}
	}`

	var rawResp upstreamTrajectoryResp
	if err := json.Unmarshal([]byte(rawJSON), &rawResp); err != nil {
		t.Fatalf("failed to unmarshal test JSON: %v", err)
	}

	p := &Proxy{}
	details := p.ParseTrajectoryDetails(&rawResp)

	if !details.CanProceed {
		t.Fatalf("expected CanProceed to be true, got false")
	}
	if details.ProceedArtifactURI != "file:///path/to/implementation_plan.md" {
		t.Fatalf("expected ProceedArtifactURI to be 'file:///path/to/implementation_plan.md', got %q", details.ProceedArtifactURI)
	}

	// Test 2: If subsequent user input exists, CanProceed should be false
	rawJSONAfterUserMessage := `{
		"status": "CASCADE_RUN_STATUS_IDLE",
		"trajectory": {
			"cascadeId": "test-cascade-123",
			"steps": [
				{
					"type": "CORTEX_STEP_TYPE_USER_INPUT",
					"status": "CORTEX_STEP_STATUS_DONE",
					"userInput": { "userResponse": "Please generate a plan" }
				},
				{
					"type": "CORTEX_STEP_TYPE_CODE_ACTION",
					"status": "CORTEX_STEP_STATUS_DONE",
					"codeAction": {
						"isArtifactFile": true,
						"artifactMetadata": { "requestFeedback": true },
						"actionResult": {
							"edit": { "absoluteUri": "file:///path/to/implementation_plan.md" }
						}
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_USER_INPUT",
					"status": "CORTEX_STEP_STATUS_DONE",
					"userInput": { "userResponse": "I have reviewed it" }
				},
				{
					"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
					"status": "CORTEX_STEP_STATUS_DONE",
					"plannerResponse": { "response": "Proceeding with changes..." }
				}
			]
		}
	}`

	var rawResp2 upstreamTrajectoryResp
	if err := json.Unmarshal([]byte(rawJSONAfterUserMessage), &rawResp2); err != nil {
		t.Fatalf("failed to unmarshal test JSON 2: %v", err)
	}

	details2 := p.ParseTrajectoryDetails(&rawResp2)
	if details2.CanProceed {
		t.Fatalf("expected CanProceed to be false after user response, got true")
	}

	// Test 3: If non-artifact code files were modified in the same turn, CanProceed should be false
	rawJSONWithCodeEdits := `{
		"status": "CASCADE_RUN_STATUS_IDLE",
		"trajectory": {
			"cascadeId": "test-cascade-123",
			"steps": [
				{
					"type": "CORTEX_STEP_TYPE_USER_INPUT",
					"status": "CORTEX_STEP_STATUS_DONE",
					"userInput": { "userResponse": "Please generate a plan" }
				},
				{
					"type": "CORTEX_STEP_TYPE_CODE_ACTION",
					"status": "CORTEX_STEP_STATUS_DONE",
					"codeAction": {
						"isArtifactFile": true,
						"artifactMetadata": { "requestFeedback": true },
						"actionResult": {
							"edit": { "absoluteUri": "file:///path/to/implementation_plan.md" }
						}
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_CODE_ACTION",
					"status": "CORTEX_STEP_STATUS_DONE",
					"codeAction": {
						"isArtifactFile": false,
						"actionResult": {
							"edit": { "absoluteUri": "file:///path/to/main.go" }
						}
					}
				}
			]
		}
	}`

	var rawResp3 upstreamTrajectoryResp
	if err := json.Unmarshal([]byte(rawJSONWithCodeEdits), &rawResp3); err != nil {
		t.Fatalf("failed to unmarshal test JSON 3: %v", err)
	}

	details3 := p.ParseTrajectoryDetails(&rawResp3)
	if details3.CanProceed {
		t.Fatalf("expected CanProceed to be false when code files were modified in same turn, got true")
	}
}

func TestParseTrajectoryDetails_PendingInteraction(t *testing.T) {
	rawJSON := `{
		"status": "CASCADE_RUN_STATUS_RUNNING",
		"trajectory": {
			"trajectoryId": "traj-test-999",
			"cascadeId": "cascade-test-999",
			"steps": [
				{
					"type": "CORTEX_STEP_TYPE_USER_INPUT",
					"status": "CORTEX_STEP_STATUS_DONE",
					"userInput": {
						"userResponse": "Read the file"
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_GENERIC",
					"status": "CORTEX_STEP_STATUS_WAITING",
					"requestedInteraction": {
						"permission": {
							"resource": {
								"action": "read_file",
								"target": "/Applications/ego lite.app/SKILL.md"
							},
							"actionDescription": "View ego-browser skill documentation"
						}
					}
				}
			]
		}
	}`

	var rawResp upstreamTrajectoryResp
	if err := json.Unmarshal([]byte(rawJSON), &rawResp); err != nil {
		t.Fatalf("failed to unmarshal test JSON: %v", err)
	}

	p := &Proxy{}
	details := p.ParseTrajectoryDetails(&rawResp)

	if details.PendingInteraction == nil {
		t.Fatalf("expected PendingInteraction to be non-nil, got nil")
	}
	pi := details.PendingInteraction
	if pi.Type != "permission" {
		t.Errorf("expected type 'permission', got %q", pi.Type)
	}
	if pi.Title != "Allow read access to this path?" {
		t.Errorf("expected title 'Allow read access to this path?', got %q", pi.Title)
	}
	if pi.Target != "/Applications/ego lite.app/SKILL.md" {
		t.Errorf("expected target '/Applications/ego lite.app/SKILL.md', got %q", pi.Target)
	}
	if len(pi.Options) != 5 {
		t.Errorf("expected 5 options, got %d", len(pi.Options))
	}
	if pi.Options[0].Text != "Yes, allow this time" {
		t.Errorf("expected first option 'Yes, allow this time', got %q", pi.Options[0].Text)
	}
}

func TestActualCascade_51dbc1ee(t *testing.T) {
	insp := inspector.NewInspector(5 * time.Second)
	info := insp.Scan()
	if info == nil {
		t.Skip("Antigravity instance not available")
	}
	p := NewProxy(insp)
	rawResp, err := p.fetchUpstreamTrajectory("51dbc1ee-33a5-4272-a5a6-468901235e0c", info.Port, info.CSRFToken)
	if err != nil {
		t.Fatalf("failed to fetch trajectory: %v", err)
	}
	details := p.ParseTrajectoryDetails(rawResp)
	t.Logf("Status: %s, PendingInteraction: %+v", details.Status, details.PendingInteraction)
	if details.Status == "CASCADE_RUN_STATUS_RUNNING" {
		if details.PendingInteraction == nil {
			t.Errorf("expected 51dbc1ee to have active PendingInteraction while waiting")
		} else {
			t.Logf("Extracted pending interaction: title=%s, target=%s, options=%d",
				details.PendingInteraction.Title, details.PendingInteraction.Target, len(details.PendingInteraction.Options))
		}
	}
}

func TestActualCascade_d363164b(t *testing.T) {
	insp := inspector.NewInspector(5 * time.Second)
	info := insp.Scan()
	if info == nil {
		t.Skip("Antigravity instance not available")
	}
	p := NewProxy(insp)
	rawResp, err := p.fetchUpstreamTrajectory("d363164b-e572-4de3-b4ad-05029eb6a629", info.Port, info.CSRFToken)
	if err != nil {
		t.Fatalf("failed to fetch trajectory: %v", err)
	}
	details := p.ParseTrajectoryDetails(rawResp)
	t.Logf("CanProceed: %t, ProceedArtifactURI: %s", details.CanProceed, details.ProceedArtifactURI)
	if details.CanProceed {
		t.Errorf("expected completed cascade d363164b to have CanProceed == false, got true")
	}
	if details.ProceedArtifactURI != "" {
		t.Errorf("expected completed cascade d363164b to have empty ProceedArtifactURI, got %q", details.ProceedArtifactURI)
	}
}
