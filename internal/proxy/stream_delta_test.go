package proxy

import (
	"reflect"
	"testing"
)

func msg(id, text string) CascadeMessageItem {
	return CascadeMessageItem{ID: id, Type: "agent", Text: text}
}

func step(typ, content string) TrajectoryStep {
	return TrajectoryStep{Type: typ, Content: content}
}

func TestStreamDeltaStateMessages(t *testing.T) {
	var d streamDeltaState

	init := StreamUpdatePayload{Type: "init", Messages: []CascadeMessageItem{msg("a", "1"), msg("b", "2")}}
	d.apply(&init)
	if init.Delta || len(init.Messages) != 2 || init.MessageIDs != nil {
		t.Fatalf("init must stay a complete frame: %+v", init)
	}

	// Only the streaming tail changed.
	u1 := StreamUpdatePayload{Type: "update", Messages: []CascadeMessageItem{msg("a", "1"), msg("b", "2 more")}}
	d.apply(&u1)
	if !u1.Delta || len(u1.Messages) != 1 || u1.Messages[0].ID != "b" {
		t.Fatalf("expected only b to be sent: %+v", u1.Messages)
	}
	if !reflect.DeepEqual(u1.MessageIDs, []string{"a", "b"}) {
		t.Fatalf("messageIds: %v", u1.MessageIDs)
	}

	// Window slides: a drops out, c is new, b unchanged.
	u2 := StreamUpdatePayload{Type: "update", Messages: []CascadeMessageItem{msg("b", "2 more"), msg("c", "3")}}
	d.apply(&u2)
	if len(u2.Messages) != 1 || u2.Messages[0].ID != "c" || !reflect.DeepEqual(u2.MessageIDs, []string{"b", "c"}) {
		t.Fatalf("slide: msgs=%+v ids=%v", u2.Messages, u2.MessageIDs)
	}

	// Same-length rewrite is still detected (content hash, not length).
	u3 := StreamUpdatePayload{Type: "update", Messages: []CascadeMessageItem{msg("b", "2 MORE"), msg("c", "3")}}
	d.apply(&u3)
	if len(u3.Messages) != 1 || u3.Messages[0].ID != "b" {
		t.Fatalf("same-length rewrite missed: %+v", u3.Messages)
	}
}

func TestStreamDeltaStateSteps(t *testing.T) {
	var d streamDeltaState
	init := StreamUpdatePayload{Type: "init", Steps: []TrajectoryStep{step("x", "1"), step("y", "2")}}
	d.apply(&init)
	if len(init.Steps) != 2 || init.StepsFrom != nil {
		t.Fatalf("init steps must be complete: %+v", init)
	}

	// Tail step grew and one was appended.
	u1 := StreamUpdatePayload{Type: "update", Steps: []TrajectoryStep{step("x", "1"), step("y", "22"), step("z", "3")}}
	d.apply(&u1)
	if u1.StepsFrom == nil || *u1.StepsFrom != 1 || len(u1.Steps) != 2 {
		t.Fatalf("expected steps[1:], got from=%v len=%d", u1.StepsFrom, len(u1.Steps))
	}

	// Nothing changed in steps (e.g. only queue changed): empty suffix at the end.
	u2 := StreamUpdatePayload{Type: "update", Steps: []TrajectoryStep{step("x", "1"), step("y", "22"), step("z", "3")}}
	d.apply(&u2)
	if u2.StepsFrom == nil || *u2.StepsFrom != 3 || len(u2.Steps) != 0 {
		t.Fatalf("expected empty suffix at 3, got from=%v len=%d", u2.StepsFrom, len(u2.Steps))
	}

	// Truncation (e.g. revert): client truncates to TotalSteps.
	u3 := StreamUpdatePayload{Type: "update", Steps: []TrajectoryStep{step("x", "1")}}
	d.apply(&u3)
	if u3.StepsFrom == nil || *u3.StepsFrom != 1 || len(u3.Steps) != 0 {
		t.Fatalf("truncate: from=%v len=%d", u3.StepsFrom, len(u3.Steps))
	}

	// messages-only clients carry no steps and must not get stepsFrom.
	var d2 streamDeltaState
	m := StreamUpdatePayload{Type: "update", Messages: []CascadeMessageItem{msg("a", "1")}}
	d2.apply(&m)
	if m.StepsFrom != nil {
		t.Fatal("stepsFrom must be unset when the payload has no steps")
	}
}

func TestFingerprintDetectsSameLengthRewrite(t *testing.T) {
	a := StreamUpdatePayload{Messages: []CascadeMessageItem{msg("a", "hello")}}
	b := StreamUpdatePayload{Messages: []CascadeMessageItem{msg("a", "world")}}
	if a.Fingerprint() == b.Fingerprint() {
		t.Fatal("message fingerprint ignored a same-length rewrite")
	}

	sa := StreamUpdatePayload{Steps: []TrajectoryStep{step("x", "hello")}}
	sb := StreamUpdatePayload{Steps: []TrajectoryStep{step("x", "world")}}
	if sa.Fingerprint() == sb.Fingerprint() {
		t.Fatal("step fingerprint ignored a same-length rewrite")
	}
}
