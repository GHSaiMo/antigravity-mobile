package proxy

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"regexp"
	"strings"
)

// 排队消息的客户端标识。
//
// SendUserCascadeMessage 不返回消息 id，客户端只能在随后的队列里按文字去猜哪条是自己刚发的。
// 实测请求里的 tags 字段会原样存进队列条目（stepPayload 内部的 user_input.tags），
// 所以网关把客户端已经在发的 X-Client-Message-Id 写成一个 tag，读队列时再解出来，
// 客户端就能用 clientMessageId 精确对应到服务端 id（用于删除/立即发送），不用再按文字匹配。

const clientTagPrefix = "mgy-client:"

var clientMessageIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// applyClientMessageTag adds the client message id as a tag on an outgoing SendUserCascadeMessage body.
// It reports whether the body changed. Invalid ids are ignored (they are only used for correlation).
func applyClientMessageTag(rawMap map[string]interface{}, clientMsgID string) bool {
	clientMsgID = strings.TrimSpace(clientMsgID)
	if !clientMessageIDRe.MatchString(clientMsgID) {
		return false
	}
	tag := clientTagPrefix + clientMsgID
	switch existing := rawMap["tags"].(type) {
	case nil:
		rawMap["tags"] = []interface{}{tag}
		return true
	case []interface{}:
		for _, t := range existing {
			if s, ok := t.(string); ok && s == tag {
				return false
			}
		}
		rawMap["tags"] = append(existing, tag)
		return true
	default:
		return false
	}
}

// stepPayloadBytes returns the decoded protobuf bytes of a pending message's stepPayload, or nil.
func stepPayloadBytes(pam upstreamAgentMessage) []byte {
	candidates := make([]json.RawMessage, 0, 2)
	if pam.Payload != nil && pam.Payload.Case == "stepPayload" && len(pam.Payload.Value) > 0 {
		candidates = append(candidates, pam.Payload.Value)
	}
	if len(pam.StepPayload) > 0 {
		candidates = append(candidates, pam.StepPayload)
	}
	for _, raw := range candidates {
		var b64 string
		if err := json.Unmarshal(raw, &b64); err != nil || b64 == "" {
			continue
		}
		if data, err := base64.StdEncoding.DecodeString(b64); err == nil && len(data) > 0 {
			return data
		}
	}
	return nil
}

// protoFields iterates the length-delimited fields of a protobuf message, skipping scalar fields.
func protoFields(data []byte, fn func(field uint64, val []byte) bool) {
	idx := 0
	for idx < len(data) {
		tag, n := binary.Uvarint(data[idx:])
		if n <= 0 {
			return
		}
		idx += n
		field, wire := tag>>3, tag&0x7
		switch wire {
		case 0:
			_, n := binary.Uvarint(data[idx:])
			if n <= 0 {
				return
			}
			idx += n
		case 1:
			idx += 8
		case 5:
			idx += 4
		case 2:
			length, n := binary.Uvarint(data[idx:])
			if n <= 0 || uint64(len(data)-idx-n) < length {
				return
			}
			idx += n
			val := data[idx : idx+int(length)]
			idx += int(length)
			if !fn(field, val) {
				return
			}
		default:
			return
		}
	}
}

// extractQueuedClientMessageID reads the mgy-client tag from a pending message (step.user_input.tags),
// or returns "" when the message was not sent through the gateway.
func extractQueuedClientMessageID(pam upstreamAgentMessage) string {
	data := stepPayloadBytes(pam)
	if len(data) == 0 {
		return ""
	}
	found := ""
	protoFields(data, func(field uint64, val []byte) bool {
		if field != 19 { // Step.user_input
			return true
		}
		protoFields(val, func(f uint64, v []byte) bool {
			if f == 16 { // UserInput.tags
				if s := string(v); strings.HasPrefix(s, clientTagPrefix) {
					if id := strings.TrimPrefix(s, clientTagPrefix); clientMessageIDRe.MatchString(id) {
						found = id
						return false
					}
				}
			}
			return true
		})
		return found == ""
	})
	return found
}
