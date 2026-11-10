package proxy

import (
	"fmt"
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

const (
	fnvOffset64 uint64 = 14695981039346656037
	fnvPrime64  uint64 = 1099511628211
)

type fnv64 uint64

func newFNV64() fnv64 { return fnv64(fnvOffset64) }

func (h *fnv64) u8(b byte) {
	*h = (*h ^ fnv64(b)) * fnv64(fnvPrime64)
}

func (h *fnv64) str(s string) {
	v := uint64(*h)
	for i := 0; i < len(s); i++ {
		v = (v ^ uint64(s[i])) * fnvPrime64
	}
	v = (v ^ 0) * fnvPrime64
	*h = fnv64(v)
}

func (h *fnv64) bytes(b []byte) {
	v := uint64(*h)
	for i := 0; i < len(b); i++ {
		v = (v ^ uint64(b[i])) * fnvPrime64
	}
	v = (v ^ 0) * fnvPrime64
	*h = fnv64(v)
}

func (h *fnv64) flag(b bool) {
	if b {
		h.u8(1)
	} else {
		h.u8(0)
	}
}

func (h *fnv64) num(n int) {
	v := uint64(*h)
	u := uint64(n)
	for i := 0; i < 8; i++ {
		v = (v ^ byteToU64(byte(u))) * fnvPrime64
		u >>= 8
	}
	*h = fnv64(v)
}

func byteToU64(b byte) uint64 { return uint64(b) }

func hashSubagentItem(h *fnv64, sa *SubagentItem) {
	if sa == nil {
		h.u8(0)
		return
	}
	h.u8(1)
	h.str(sa.ConversationID)
	h.str(sa.TypeName)
	h.str(sa.Role)
	h.str(sa.Prompt)
	h.str(sa.ModelTier)
	h.num(sa.StepIndex)
	h.str(sa.Status)
	h.num(sa.StepCount)
	h.str(sa.Title)
}

func hashMessageItem(m *CascadeMessageItem) uint64 {
	h := newFNV64()
	h.str(m.ID)
	h.str(m.Type)
	h.str(m.Role)
	h.str(m.Text)
	h.str(m.Content)
	if m.StepIndex != nil {
		h.u8(1)
		h.num(*m.StepIndex)
	} else {
		h.u8(0)
	}
	h.num(m.ToolCount)
	h.num(len(m.ToolNames))
	for _, tn := range m.ToolNames {
		h.str(tn)
	}
	h.num(len(m.Media))
	for _, md := range m.Media {
		h.str(md)
	}
	h.num(len(m.ImageURLs))
	for _, u := range m.ImageURLs {
		h.str(u)
	}
	h.num(len(m.Artifacts))
	for _, a := range m.Artifacts {
		h.str(a.URI)
		h.str(a.Title)
		h.str(a.Summary)
		h.flag(a.RequestFeedback)
		h.flag(a.UserFacing)
	}
	h.num(len(m.SlashCommands))
	for _, sc := range m.SlashCommands {
		h.str(sc.Name)
		h.str(sc.Icon)
		h.str(sc.Kind)
	}
	h.num(m.AttemptCount)
	h.num(m.MaxAttempts)
	h.str(m.Model)
	h.str(m.ModelName)
	hashSubagentItem(&h, m.Subagent)
	if m.Workflow != nil {
		h.u8(1)
		h.str(m.Workflow.Description)
		h.str(m.Workflow.ScriptPath)
		h.str(m.Workflow.Script)
		h.str(m.Workflow.Output)
		h.str(m.Workflow.ResultJSON)
		h.str(m.Workflow.Status)
		h.str(m.Workflow.Duration)
		h.num(len(m.Workflow.Logs))
		for _, lg := range m.Workflow.Logs {
			h.str(lg)
		}
		h.num(len(m.Workflow.Phases))
		for i := range m.Workflow.Phases {
			ph := &m.Workflow.Phases[i]
			h.str(ph.Name)
			h.str(ph.Status)
			h.num(len(ph.Logs))
			for _, lg := range ph.Logs {
				h.str(lg)
			}
			h.num(len(ph.Subagents))
			for j := range ph.Subagents {
				hashSubagentItem(&h, &ph.Subagents[j])
			}
		}
	} else {
		h.u8(0)
	}
	h.str(m.Timestamp)
	h.str(m.ClientMessageID)
	return uint64(h)
}

func hashStep(s *TrajectoryStep) uint64 {
	h := newFNV64()
	h.str(s.Type)
	h.str(s.Status)
	h.str(s.Content)
	h.str(s.Metadata.CreatedAt)
	h.str(s.Metadata.GeneratorModel)
	h.str(s.Metadata.ToolSummary)
	h.str(s.Metadata.ToolAction)
	if s.Metadata.SourceTrajectoryStepInfo != nil {
		h.u8(1)
		h.num(s.Metadata.SourceTrajectoryStepInfo.StepIndex)
	} else {
		h.u8(0)
	}
	if s.Metadata.ToolCall != nil {
		h.u8(1)
		h.str(s.Metadata.ToolCall.ID)
		h.str(s.Metadata.ToolCall.Name)
		h.str(s.Metadata.ToolCall.ArgumentsJson)
	} else {
		h.u8(0)
	}
	if s.TaskDetails != nil {
		h.u8(1)
		h.str(s.TaskDetails.ID)
		h.str(s.TaskDetails.LogURI)
		h.str(s.TaskDetails.Description)
		h.flag(s.TaskDetails.IsDaemon)
	} else {
		h.u8(0)
	}
	if s.RunCommand != nil {
		h.u8(1)
		h.str(s.RunCommand.CommandLine)
		h.str(s.RunCommand.ProposedCommandLine)
		h.str(s.RunCommand.Cwd)
		h.str(s.RunCommand.WaitMsBeforeAsync)
		h.flag(s.RunCommand.IsDaemon)
	} else {
		h.u8(0)
	}
	if s.InvokeSubagent != nil {
		h.u8(1)
		h.num(len(s.InvokeSubagent.Subagents))
		for i := range s.InvokeSubagent.Subagents {
			sub := &s.InvokeSubagent.Subagents[i]
			h.str(sub.TypeName)
			h.str(sub.Role)
			h.str(sub.InitialPrompt)
			h.str(sub.ModelTier)
		}
		h.num(len(s.InvokeSubagent.Results))
		for i := range s.InvokeSubagent.Results {
			h.str(s.InvokeSubagent.Results[i].ConversationID)
		}
	} else {
		h.u8(0)
	}
	if s.UserInput != nil {
		h.u8(1)
		h.str(s.UserInput.UserResponse)
		h.str(s.UserInput.ClientMessageID)
		h.num(len(s.UserInput.Items))
		for i := range s.UserInput.Items {
			it := &s.UserInput.Items[i]
			h.str(it.Text)
			if it.Item != nil && it.Item.SlashCommand != nil && it.Item.SlashCommand.Info != nil {
				h.u8(1)
				h.str(it.Item.SlashCommand.Info.Name)
				h.str(it.Item.SlashCommand.Info.Type)
				h.str(it.Item.SlashCommand.Info.Icon)
			} else {
				h.u8(0)
			}
		}
		h.num(len(s.UserInput.Images))
		for i := range s.UserInput.Images {
			h.str(s.UserInput.Images[i].Base64Data)
			h.str(s.UserInput.Images[i].MimeType)
		}
		h.num(len(s.UserInput.Media))
		for i := range s.UserInput.Media {
			md := &s.UserInput.Media[i]
			h.str(md.MimeType)
			h.str(md.Description)
			h.str(md.Thumbnail)
			h.str(md.InlineData)
			h.str(md.URI)
		}
		h.num(len(s.UserInput.ArtifactComments))
		for i := range s.UserInput.ArtifactComments {
			ac := &s.UserInput.ArtifactComments[i]
			h.str(ac.ArtifactURI)
			h.str(fmt.Sprint(ac.ApprovalStatus))
			h.str(ac.Comment)
		}
		h.num(len(s.UserInput.Tags))
		for _, tg := range s.UserInput.Tags {
			h.str(tg)
		}
	} else {
		h.u8(0)
	}
	if s.PlannerResponse != nil {
		h.u8(1)
		h.str(s.PlannerResponse.Response)
		h.str(s.PlannerResponse.Thinking)
	} else {
		h.u8(0)
	}
	if s.CodeAction != nil {
		h.u8(1)
		h.flag(s.CodeAction.IsArtifactFile)
		if s.CodeAction.ArtifactMetadata != nil {
			h.u8(1)
			h.str(s.CodeAction.ArtifactMetadata.Summary)
			h.flag(s.CodeAction.ArtifactMetadata.RequestFeedback)
			if s.CodeAction.ArtifactMetadata.UserFacing != nil {
				h.u8(1)
				h.flag(*s.CodeAction.ArtifactMetadata.UserFacing)
			} else {
				h.u8(0)
			}
		} else {
			h.u8(0)
		}
		if s.CodeAction.ActionResult != nil {
			h.u8(1)
			h.str(s.CodeAction.ActionResult.AbsoluteURI)
			if s.CodeAction.ActionResult.Edit != nil {
				h.u8(1)
				h.str(s.CodeAction.ActionResult.Edit.AbsoluteURI)
				h.flag(s.CodeAction.ActionResult.Edit.CreateFile)
			} else {
				h.u8(0)
			}
		} else {
			h.u8(0)
		}
		if s.CodeAction.ActionSpec != nil && s.CodeAction.ActionSpec.CreateFile != nil && s.CodeAction.ActionSpec.CreateFile.Path != nil {
			h.u8(1)
			h.str(s.CodeAction.ActionSpec.CreateFile.Path.AbsoluteURI)
		} else {
			h.u8(0)
		}
	} else {
		h.u8(0)
	}
	if s.RequestedInteraction != nil {
		h.u8(1)
		ri := s.RequestedInteraction
		if ri.Permission != nil {
			h.u8(1)
			h.str(ri.Permission.Resource.Action)
			h.str(ri.Permission.Resource.Target)
			h.str(ri.Permission.ActionDescription)
		} else {
			h.u8(0)
		}
		if ri.AskQuestion != nil {
			h.u8(1)
			h.num(len(ri.AskQuestion.Questions))
			for i := range ri.AskQuestion.Questions {
				q := &ri.AskQuestion.Questions[i]
				h.str(q.Question)
				h.flag(q.IsMultiSelect)
				h.num(len(q.Options))
				for j := range q.Options {
					h.str(q.Options[j].ID)
					h.str(q.Options[j].Text)
				}
			}
		} else {
			h.u8(0)
		}
		if ri.RunCommand != nil {
			h.u8(1)
			h.str(ri.RunCommand.CommandLine)
		} else {
			h.u8(0)
		}
		if ri.FilePermission != nil {
			h.u8(1)
			h.str(ri.FilePermission.AbsolutePathURI)
		} else {
			h.u8(0)
		}
	} else {
		h.u8(0)
	}
	if s.ErrorMessage != nil {
		h.u8(1)
		e := &s.ErrorMessage.Error
		h.str(e.UserErrorMessage)
		h.str(e.ModelErrorMessage)
		h.str(e.ShortError)
		h.str(e.FullError)
		h.num(e.ErrorCode)
		h.str(e.ErrorID)
		h.flag(e.IsBenign)
		h.flag(s.ErrorMessage.ShouldShowUser)
		h.flag(s.ErrorMessage.ShouldShowModel)
	} else {
		h.u8(0)
	}
	if s.Error != nil {
		h.u8(1)
		h.str(s.Error.ShortError)
		h.str(s.Error.FullError)
	} else {
		h.u8(0)
	}
	if s.SystemMessage != nil {
		h.u8(1)
		h.str(s.SystemMessage.Content)
	} else {
		h.u8(0)
	}
	if s.ToolCall != nil {
		h.u8(1)
		h.str(s.ToolCall.ID)
		h.str(s.ToolCall.Name)
		h.str(s.ToolCall.ToolSummary)
		h.str(s.ToolCall.ToolAction)
		h.str(s.ToolCall.ArgumentsJson)
	} else {
		h.u8(0)
	}
	return uint64(h)
}

// apply records the payload as the client's new baseline and, for non-init payloads,
// rewrites it into delta form.
func (d *streamDeltaState) apply(p *StreamUpdatePayload) {
	newMsgSigs := make(map[string]uint64, len(p.Messages))
	ids := make([]string, 0, len(p.Messages))
	changed := make([]CascadeMessageItem, 0, 2)
	for i := range p.Messages {
		m := &p.Messages[i]
		sig := hashMessageItem(m)
		newMsgSigs[m.ID] = sig
		ids = append(ids, m.ID)
		if prev, ok := d.msgSigs[m.ID]; !ok || prev != sig {
			changed = append(changed, *m)
		}
	}

	newStepSigs := make([]uint64, len(p.Steps))
	from := len(p.Steps)
	for i := range p.Steps {
		newStepSigs[i] = hashStep(&p.Steps[i])
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

// hashStrings returns a zero-allocation FNV-1a hash over the strings, separated so ("ab","c") != ("a","bc").
func hashStrings(parts ...string) uint64 {
	h := newFNV64()
	for _, s := range parts {
		h.str(s)
	}
	return uint64(h)
}
