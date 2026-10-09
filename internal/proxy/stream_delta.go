package proxy

import (
	"encoding/json"
	"hash/fnv"
)

// streamDeltaState remembers what one WebSocket client has already been sent so that,
// for clients that opt in with `delta=1`, "update" frames can carry only what changed.
//
// Wire contract for delta frames (Type == "update", Delta == true):
//   - Messages holds only new or modified messages; MessageIDs is the complete ordered ID list
//     of the current window, so the client rebuilds the window from its local copy and drops
//     IDs that are no longer listed.
//   - Steps (non-messages-only clients) holds steps[StepsFrom:]; the client keeps its first
//     StepsFrom steps, appends Steps, and truncates to TotalSteps.
//
// "init" frames are always complete and reset the client's baseline. All other fields are
// sent in full on every frame. Not safe for concurrent use; owned by one stream goroutine.
type streamDeltaState struct {
	msgSigs  map[string]uint64
	stepSigs []uint64
}

func hashJSON(v interface{}) uint64 {
	b, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	h := fnv.New64a()
	_, _ = h.Write(b)
	return h.Sum64()
}

// apply records the payload as the client's new baseline and, for non-init payloads,
// rewrites it into delta form.
func (d *streamDeltaState) apply(p *StreamUpdatePayload) {
	newMsgSigs := make(map[string]uint64, len(p.Messages))
	ids := make([]string, 0, len(p.Messages))
	changed := make([]CascadeMessageItem, 0, 2)
	for _, m := range p.Messages {
		sig := hashJSON(m)
		newMsgSigs[m.ID] = sig
		ids = append(ids, m.ID)
		if prev, ok := d.msgSigs[m.ID]; !ok || prev != sig {
			changed = append(changed, m)
		}
	}

	newStepSigs := make([]uint64, len(p.Steps))
	from := len(p.Steps)
	for i := range p.Steps {
		newStepSigs[i] = hashJSON(p.Steps[i])
		if from == len(p.Steps) && (i >= len(d.stepSigs) || d.stepSigs[i] != newStepSigs[i]) {
			from = i
		}
	}

	isInit := p.Type == "init"
	d.msgSigs, d.stepSigs = newMsgSigs, newStepSigs
	if isInit {
		return
	}

	p.Delta = true
	p.MessageIDs = ids
	p.Messages = changed
	if p.Steps != nil {
		p.Steps = p.Steps[from:]
		p.StepsFrom = &from
	}
}

// hashStrings returns an FNV-1a hash over the strings, separated so ("ab","c") != ("a","bc").
func hashStrings(parts ...string) uint64 {
	h := fnv.New64a()
	for _, s := range parts {
		_, _ = h.Write([]byte(s))
		_, _ = h.Write([]byte{0})
	}
	return h.Sum64()
}
