package cockpit

import (
	"encoding/base64"
	"errors"
	"fmt"
)

// encodeVarint encodes an unsigned 64-bit integer into protobuf varint format.
func encodeVarint(val uint64) []byte {
	var buf []byte
	for val >= 0x80 {
		buf = append(buf, byte(val&0x7F|0x80))
		val >>= 7
	}
	buf = append(buf, byte(val))
	return buf
}

// readVarint decodes a protobuf varint from data starting at offset.
func readVarint(data []byte, offset int) (uint64, int, error) {
	var result uint64
	var shift uint
	pos := offset

	for {
		if pos >= len(data) {
			return 0, pos, errors.New("incomplete protobuf varint data")
		}
		b := data[pos]
		result |= uint64(b&0x7F) << shift
		pos++
		if b&0x80 == 0 {
			break
		}
		shift += 7
		if shift >= 64 {
			return 0, pos, errors.New("protobuf varint overflow")
		}
	}
	return result, pos, nil
}

// skipField calculates the offset after skipping a field given its wire type.
func skipField(data []byte, offset int, wireType byte) (int, error) {
	switch wireType {
	case 0: // Varint
		_, newOffset, err := readVarint(data, offset)
		return newOffset, err
	case 1: // 64-bit
		if offset+8 > len(data) {
			return 0, errors.New("incomplete 64-bit field")
		}
		return offset + 8, nil
	case 2: // Length-delimited
		length, contentOffset, err := readVarint(data, offset)
		if err != nil {
			return 0, err
		}
		end := contentOffset + int(length)
		if end > len(data) {
			return 0, errors.New("incomplete length-delimited field")
		}
		return end, nil
	case 5: // 32-bit
		if offset+4 > len(data) {
			return 0, errors.New("incomplete 32-bit field")
		}
		return offset + 4, nil
	default:
		return 0, fmt.Errorf("unknown wire type: %d", wireType)
	}
}

// encodeLenDelimField encodes a field with wire_type = 2 (length-delimited).
func encodeLenDelimField(fieldNum uint32, data []byte) []byte {
	tag := (fieldNum << 3) | 2
	var f []byte
	f = append(f, encodeVarint(uint64(tag))...)
	f = append(f, encodeVarint(uint64(len(data)))...)
	f = append(f, data...)
	return f
}

// encodeStringField encodes a string field (wire_type = 2).
func encodeStringField(fieldNum uint32, val string) []byte {
	return encodeLenDelimField(fieldNum, []byte(val))
}

// encodeVarintField encodes a uint64 field (wire_type = 0).
func encodeVarintField(fieldNum uint32, val uint64) []byte {
	tag := (fieldNum << 3) | 0
	var f []byte
	f = append(f, encodeVarint(uint64(tag))...)
	f = append(f, encodeVarint(val)...)
	return f
}

// CreateOAuthInfoWithMetadata encodes the OAuthTokenInfo protobuf payload required by Antigravity IDE.
func CreateOAuthInfoWithMetadata(accessToken, refreshToken string, expiry int64, isGCPToS bool, idToken string) []byte {
	var oauthInfo []byte

	// Field 1: access_token (string)
	oauthInfo = append(oauthInfo, encodeStringField(1, accessToken)...)

	// Field 2: token_type ("Bearer")
	oauthInfo = append(oauthInfo, encodeStringField(2, "Bearer")...)

	// Field 3: refresh_token (string)
	oauthInfo = append(oauthInfo, encodeStringField(3, refreshToken)...)

	// Field 4: expiry (nested Timestamp message: field 1: seconds, field 2: nanos)
	timestampTag := (1 << 3) | 0
	var timestampMsg []byte
	timestampMsg = append(timestampMsg, encodeVarint(uint64(timestampTag))...)
	timestampMsg = append(timestampMsg, encodeVarint(uint64(expiry))...)
	timestampMsg = append(timestampMsg, encodeVarintField(2, 0)...)

	oauthInfo = append(oauthInfo, encodeLenDelimField(4, timestampMsg)...)

	// Field 5: id_token (optional)
	if idToken != "" {
		oauthInfo = append(oauthInfo, encodeStringField(5, idToken)...)
	}

	// Field 6: is_gcp_tos (bool as varint)
	if isGCPToS {
		oauthInfo = append(oauthInfo, encodeVarintField(6, 1)...)
	}

	return oauthInfo
}

// CreateUnifiedTopicEntry creates a unified topic map entry for Antigravity state.vscdb.
func CreateUnifiedTopicEntry(sentinelKey string, payload []byte) []byte {
	row := encodeStringField(1, base64.StdEncoding.EncodeToString(payload))
	entry := append(encodeStringField(1, sentinelKey), encodeLenDelimField(2, row)...)
	return encodeLenDelimField(1, entry)
}

// CreateMinimalUserStatusPayload creates the minimal user status binary containing the email.
func CreateMinimalUserStatusPayload(email string) []byte {
	return append(encodeStringField(3, email), encodeStringField(7, email)...)
}

// RemoveUnifiedTopicEntry removes a sentinel row from the Topic.data binary.
func RemoveUnifiedTopicEntry(data []byte, targetKey string) ([]byte, error) {
	var result []byte
	offset := 0

	for offset < len(data) {
		startOffset := offset
		tag, newOffset, err := readVarint(data, offset)
		if err != nil {
			return nil, err
		}
		wireType := byte(tag & 7)
		fieldNum := uint32(tag >> 3)
		nextOffset, err := skipField(data, newOffset, wireType)
		if err != nil {
			return nil, err
		}

		shouldRemove := false
		if fieldNum == 1 && wireType == 2 {
			length, contentOffset, err := readVarint(data, newOffset)
			if err == nil {
				end := contentOffset + int(length)
				if end <= len(data) {
					entry := data[contentOffset:end]
					if unifiedTopicEntryKey(entry) == targetKey {
						shouldRemove = true
					}
				}
			}
		}

		if !shouldRemove {
			result = append(result, data[startOffset:nextOffset]...)
		}
		offset = nextOffset
	}

	return result, nil
}

func unifiedTopicEntryKey(data []byte) string {
	offset := 0
	for offset < len(data) {
		tag, newOffset, err := readVarint(data, offset)
		if err != nil {
			return ""
		}
		wireType := byte(tag & 7)
		fieldNum := uint32(tag >> 3)

		if fieldNum == 1 && wireType == 2 {
			length, contentOffset, err := readVarint(data, newOffset)
			if err != nil {
				return ""
			}
			end := contentOffset + int(length)
			if end > len(data) {
				return ""
			}
			return string(data[contentOffset:end])
		}

		nextOffset, err := skipField(data, newOffset, wireType)
		if err != nil {
			return ""
		}
		offset = nextOffset
	}
	return ""
}
