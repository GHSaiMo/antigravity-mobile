package proxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// upstreamAgentMessage represents a pending message received from StreamAgentStateUpdates.
type upstreamAgentMessage struct {
	ID               string          `json:"id"`
	Recipient        string          `json:"recipient"`
	Sender           string          `json:"sender"`
	Timestamp        interface{}     `json:"timestamp"`
	Content          string          `json:"content"`
	StepPayload      json.RawMessage `json:"stepPayload"`
	DeliveryStrategy interface{}     `json:"deliveryStrategy"`
	Payload          *struct {
		Case  string          `json:"case"`
		Value json.RawMessage `json:"value"`
	} `json:"payload"`
}

// upstreamAgentStateUpdate represents the envelope returned by StreamAgentStateUpdates.
type upstreamAgentStateUpdate struct {
	Update struct {
		ConversationID             string `json:"conversationId"`
		TrajectoryID               string `json:"trajectoryId"`
		Status                     string `json:"status"`
		PendingAgentMessagesUpdate *struct {
			Indices              []uint32               `json:"indices"`
			PendingAgentMessages []upstreamAgentMessage `json:"pendingAgentMessages"`
			TotalLength          uint32                 `json:"totalLength"`
		} `json:"pendingAgentMessagesUpdate"`
		PendingAgentMessages []upstreamAgentMessage `json:"pendingAgentMessages"`
	} `json:"update"`
}

// pendingMessagesCacheEntry stores cached queued messages for a cascade.
type pendingMessagesCacheEntry struct {
	fetchedAt time.Time
	messages  []QueuedMessageItem
}

var (
	pendingCacheMu sync.RWMutex
	pendingCache   = make(map[string]*pendingMessagesCacheEntry)
	pendingTTL     = 300 * time.Millisecond
)

// ClearPendingMessagesCache clears the pending messages cache for a cascade, or all if empty.
func ClearPendingMessagesCache(cascadeID string) {
	pendingCacheMu.Lock()
	defer pendingCacheMu.Unlock()
	if cascadeID == "" {
		pendingCache = make(map[string]*pendingMessagesCacheEntry)
	} else {
		delete(pendingCache, cascadeID)
	}
}

// encodeConnectEnvelope wraps payload in a Connect streaming frame (1 byte flag + 4 bytes big-endian length).
func encodeConnectEnvelope(data []byte) []byte {
	buf := make([]byte, 5+len(data))
	buf[0] = 0 // Message flag
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(data)))
	copy(buf[5:], data)
	return buf
}

// readConnectEnvelope reads a single Connect streaming envelope.
func readConnectEnvelope(r io.Reader) (byte, []byte, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(r, header); err != nil {
		return 0, nil, err
	}
	flag := header[0]
	length := binary.BigEndian.Uint32(header[1:5])
	if length > 10*1024*1024 {
		return 0, nil, fmt.Errorf("connect envelope too large: %d bytes", length)
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return 0, nil, err
	}
	return flag, body, nil
}

// isQueuedDeliveryStrategy checks whether a deliveryStrategy represents WHEN_IDLE or pending execution.
func isQueuedDeliveryStrategy(val interface{}) bool {
	if val == nil {
		return true // Default in upstream is WHEN_IDLE
	}
	switch v := val.(type) {
	case float64:
		// 0: UNSPECIFIED (treated as WHEN_IDLE), 2: WHEN_IDLE
		return int(v) == 2 || int(v) == 0
	case int:
		return v == 2 || v == 0
	case string:
		vUpper := strings.ToUpper(v)
		if strings.Contains(vUpper, "NEXT_INVOCATION") {
			return false
		}
		return true
	default:
		return true
	}
}

// parseAgentMessageTimestamp formats the timestamp into RFC3339 string.
func parseAgentMessageTimestamp(ts interface{}) string {
	if ts == nil {
		return time.Now().UTC().Format(time.RFC3339)
	}
	switch v := ts.(type) {
	case string:
		if v != "" {
			return v
		}
	case map[string]interface{}:
		if secVal, ok := v["seconds"]; ok {
			var sec int64
			switch s := secVal.(type) {
			case float64:
				sec = int64(s)
			case int64:
				sec = s
			}
			var nsec int64
			if nanoVal, ok := v["nanos"].(float64); ok {
				nsec = int64(nanoVal)
			}
			return time.Unix(sec, nsec).UTC().Format(time.RFC3339)
		}
	}
	return time.Now().UTC().Format(time.RFC3339)
}

// extractQueuedMessageText extracts the prompt text from an upstreamAgentMessage.
func extractQueuedMessageText(pam upstreamAgentMessage) string {
	text := strings.TrimSpace(pam.Content)
	if text != "" {
		return text
	}

	rawCandidates := make([]json.RawMessage, 0, 2)
	if pam.Payload != nil {
		if pam.Payload.Case == "content" && len(pam.Payload.Value) > 0 {
			var strContent string
			if err := json.Unmarshal(pam.Payload.Value, &strContent); err == nil && strings.TrimSpace(strContent) != "" {
				return strings.TrimSpace(strContent)
			}
		} else if pam.Payload.Case == "stepPayload" && len(pam.Payload.Value) > 0 {
			rawCandidates = append(rawCandidates, pam.Payload.Value)
		}
	}
	if len(pam.StepPayload) > 0 {
		rawCandidates = append(rawCandidates, pam.StepPayload)
	}

	for _, raw := range rawCandidates {
		// 1. Try decoding as base64-encoded protobuf binary (ProtoJSON bytes field format)
		var b64Str string
		if err := json.Unmarshal(raw, &b64Str); err == nil && len(b64Str) > 0 {
			if protoBytes, err := base64.StdEncoding.DecodeString(b64Str); err == nil && len(protoBytes) > 0 {
				if t := extractTextFromProto(protoBytes, 0); t != "" {
					return t
				}
			}
		}

		// 2. Try Protobuf-ES Step schema: { "step": { "case": "userInput", "value": { "items": [...] } } }
		var esPayload struct {
			Step struct {
				Case  string `json:"case"`
				Value struct {
					Items []struct {
						Text  string `json:"text"`
						Chunk *struct {
							Case  string `json:"case"`
							Value string `json:"value"`
						} `json:"chunk"`
					} `json:"items"`
				} `json:"value"`
			} `json:"step"`
		}
		if err := json.Unmarshal(raw, &esPayload); err == nil && len(esPayload.Step.Value.Items) > 0 {
			var b strings.Builder
			for _, it := range esPayload.Step.Value.Items {
				if it.Text != "" {
					b.WriteString(it.Text)
				} else if it.Chunk != nil && it.Chunk.Value != "" {
					b.WriteString(it.Chunk.Value)
				}
			}
			if t := strings.TrimSpace(b.String()); t != "" {
				return t
			}
		}

		// Try ProtoJSON UserInput schema: { "userInput": { "items": [...] } }
		var protoPayload struct {
			UserInput struct {
				UserResponse string `json:"userResponse"`
				Response     string `json:"response"`
				Items        []struct {
					Text  string `json:"text"`
					Chunk *struct {
						Value string `json:"value"`
					} `json:"chunk"`
				} `json:"items"`
			} `json:"userInput"`
			Items []struct {
				Text  string `json:"text"`
				Chunk *struct {
					Value string `json:"value"`
				} `json:"chunk"`
			} `json:"items"`
		}
		if err := json.Unmarshal(raw, &protoPayload); err == nil {
			if strings.TrimSpace(protoPayload.UserInput.UserResponse) != "" {
				return strings.TrimSpace(protoPayload.UserInput.UserResponse)
			}
			if strings.TrimSpace(protoPayload.UserInput.Response) != "" {
				return strings.TrimSpace(protoPayload.UserInput.Response)
			}
			var b strings.Builder
			for _, it := range protoPayload.UserInput.Items {
				if it.Text != "" {
					b.WriteString(it.Text)
				} else if it.Chunk != nil && it.Chunk.Value != "" {
					b.WriteString(it.Chunk.Value)
				}
			}
			for _, it := range protoPayload.Items {
				if it.Text != "" {
					b.WriteString(it.Text)
				} else if it.Chunk != nil && it.Chunk.Value != "" {
					b.WriteString(it.Chunk.Value)
				}
			}
			if t := strings.TrimSpace(b.String()); t != "" {
				return t
			}
		}

		// Generic map traversal fallback
		var genericMap map[string]interface{}
		if err := json.Unmarshal(raw, &genericMap); err == nil {
			if t := findTextInGenericMap(genericMap); t != "" {
				return t
			}
		}
	}

	return ""
}

// findTextInGenericMap finds user input text from an arbitrary JSON map.
func findTextInGenericMap(m map[string]interface{}) string {
	if items, ok := m["items"].([]interface{}); ok {
		var b strings.Builder
		for _, it := range items {
			if itMap, ok := it.(map[string]interface{}); ok {
				if t, ok := itMap["text"].(string); ok && t != "" {
					b.WriteString(t)
				}
			}
		}
		if res := strings.TrimSpace(b.String()); res != "" {
			return res
		}
	}
	if userResp, ok := m["userResponse"].(string); ok && strings.TrimSpace(userResp) != "" {
		return strings.TrimSpace(userResp)
	}
	for _, v := range m {
		if subMap, ok := v.(map[string]interface{}); ok {
			if res := findTextInGenericMap(subMap); res != "" {
				return res
			}
		}
	}
	return ""
}

// extractTextFromProto extracts user input text from raw protobuf bytes (e.g. Step/UserInput).
func extractTextFromProto(data []byte, depth int) string {
	if depth > 6 {
		return ""
	}
	idx := 0
	for idx < len(data) {
		tag, n := binary.Uvarint(data[idx:])
		if n <= 0 {
			break
		}
		idx += n
		fieldNum := tag >> 3
		wireType := tag & 0x7
		if wireType == 2 {
			length, n := binary.Uvarint(data[idx:])
			if n <= 0 {
				break
			}
			idx += n
			if idx+int(length) > len(data) {
				break
			}
			val := data[idx : idx+int(length)]
			idx += int(length)

			// field 19 is user_input in Step
			if fieldNum == 19 {
				if t := extractTextFromProto(val, depth+1); t != "" {
					return t
				}
			}
			// field 2 of user_input is user_response
			if depth > 0 && fieldNum == 2 && utf8.Valid(val) && len(val) > 0 {
				s := string(val)
				if !strings.ContainsRune(s, 0) {
					return strings.TrimSpace(s)
				}
			}
			// field 3 of user_input is items, field 1 of item is text
			if depth > 0 && (fieldNum == 3 || fieldNum == 1) {
				if depth >= 2 && fieldNum == 1 && utf8.Valid(val) && len(val) > 0 {
					s := string(val)
					if !strings.ContainsRune(s, 0) {
						return strings.TrimSpace(s)
					}
				}
				if t := extractTextFromProto(val, depth+1); t != "" {
					return t
				}
			}
			if depth == 0 {
				if t := extractTextFromProto(val, depth+1); t != "" {
					return t
				}
			}
		} else if wireType == 0 {
			_, n := binary.Uvarint(data[idx:])
			if n <= 0 {
				break
			}
			idx += n
		} else if wireType == 1 {
			idx += 8
		} else if wireType == 5 {
			idx += 4
		} else {
			break
		}
	}
	return ""
}

// parseAgentStateQueuedMessages parses QueuedMessageItem list from upstreamAgentStateUpdate.
func parseAgentStateQueuedMessages(state *upstreamAgentStateUpdate) []QueuedMessageItem {
	if state == nil {
		return []QueuedMessageItem{}
	}

	var rawList []upstreamAgentMessage
	if state.Update.PendingAgentMessagesUpdate != nil && len(state.Update.PendingAgentMessagesUpdate.PendingAgentMessages) > 0 {
		rawList = state.Update.PendingAgentMessagesUpdate.PendingAgentMessages
	} else if len(state.Update.PendingAgentMessages) > 0 {
		rawList = state.Update.PendingAgentMessages
	}

	items := make([]QueuedMessageItem, 0, len(rawList))
	for _, pam := range rawList {
		if !isQueuedDeliveryStrategy(pam.DeliveryStrategy) {
			continue
		}
		text := extractQueuedMessageText(pam)
		if text == "" {
			continue
		}
		createdAt := parseAgentMessageTimestamp(pam.Timestamp)
		items = append(items, QueuedMessageItem{
			ID:        pam.ID,
			Text:      text,
			CreatedAt: createdAt,
		})
	}
	return items
}

// fetchUpstreamPendingMessages connects to StreamAgentStateUpdates and reads the initial agent state snapshot.
func (p *Proxy) fetchUpstreamPendingMessages(cascadeID string, port int, token string) ([]QueuedMessageItem, error) {
	if cascadeID == "" || port <= 0 {
		return []QueuedMessageItem{}, nil
	}

	reqPayload := map[string]interface{}{
		"conversationId": cascadeID,
		"subscriberId":   fmt.Sprintf("gateway-probe-%s-%d", cascadeID, time.Now().UnixNano()),
		// Limit bounds to 0 to avoid transferring heavy trajectory history; only agent state & pending messages needed
		"initialStepsPageBounds":              map[string]int{"startIndex": 0, "endIndexExclusive": 0},
		"initialGeneratorMetadatasPageBounds": map[string]int{"startIndex": 0, "endIndexExclusive": 0},
		"initialExecutorMetadatasPageBounds":  map[string]int{"startIndex": 0, "endIndexExclusive": 0},
	}

	reqBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, err
	}

	envelope := encodeConnectEnvelope(reqBytes)
	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/StreamAgentStateUpdates", port)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(envelope))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/connect+json")
	httpReq.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		httpReq.Header.Set("x-codeium-csrf-token", token)
	}

	resp, err := p.shortClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("StreamAgentStateUpdates returned %d: %s", resp.StatusCode, string(b))
	}

	flag, body, err := readConnectEnvelope(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read Connect frame from StreamAgentStateUpdates: %w", err)
	}
	if flag != 0 {
		// Flag 2 indicates end of stream without data
		return []QueuedMessageItem{}, nil
	}

	var state upstreamAgentStateUpdate
	if err := json.Unmarshal(body, &state); err != nil {
		return nil, fmt.Errorf("failed to decode AgentStateUpdate: %w", err)
	}

	items := parseAgentStateQueuedMessages(&state)
	return items, nil
}

// GetCachedOrFetchPendingMessages returns cached queued messages if fresh, or queries upstream language_server.
func (p *Proxy) GetCachedOrFetchPendingMessages(cascadeID string, port int, token string) []QueuedMessageItem {
	if cascadeID == "" || port <= 0 {
		return []QueuedMessageItem{}
	}

	pendingCacheMu.RLock()
	entry, ok := pendingCache[cascadeID]
	if ok && time.Since(entry.fetchedAt) < pendingTTL {
		pendingCacheMu.RUnlock()
		return entry.messages
	}
	pendingCacheMu.RUnlock()

	items, err := p.fetchUpstreamPendingMessages(cascadeID, port, token)
	if err != nil {
		// Return stale cache if available upon error
		pendingCacheMu.RLock()
		if entry != nil {
			cached := entry.messages
			pendingCacheMu.RUnlock()
			return cached
		}
		pendingCacheMu.RUnlock()
		return []QueuedMessageItem{}
	}

	pendingCacheMu.Lock()
	pendingCache[cascadeID] = &pendingMessagesCacheEntry{
		fetchedAt: time.Now(),
		messages:  items,
	}
	pendingCacheMu.Unlock()

	return items
}
