package cockpit

import (
	"bytes"
	"testing"
)

func TestProtobuf_VarintRoundtrip(t *testing.T) {
	cases := []uint64{0, 1, 127, 128, 300, 16384, 1791000000}
	for _, tc := range cases {
		enc := encodeVarint(tc)
		dec, pos, err := readVarint(enc, 0)
		if err != nil {
			t.Fatalf("readVarint(%d) failed: %v", tc, err)
		}
		if dec != tc {
			t.Fatalf("value mismatch: got %d, want %d", dec, tc)
		}
		if pos != len(enc) {
			t.Fatalf("pos mismatch: got %d, want %d", pos, len(enc))
		}
	}
}

func TestProtobuf_OAuthInfoAndUnifiedTopic(t *testing.T) {
	oauthInfo := CreateOAuthInfoWithMetadata(
		"ya29.access-token",
		"1//refresh-token",
		1791000000,
		true,
		"eyJ.id.token",
	)
	if len(oauthInfo) == 0 {
		t.Fatal("expected non-empty oauthInfo")
	}

	entry1 := CreateUnifiedTopicEntry("oauthTokenInfoSentinelKey", oauthInfo)
	entry2 := CreateUnifiedTopicEntry("anotherSentinelKey", []byte("sample-data"))

	combined := append(entry1, entry2...)

	// Remove oauthTokenInfoSentinelKey
	filtered, err := RemoveUnifiedTopicEntry(combined, "oauthTokenInfoSentinelKey")
	if err != nil {
		t.Fatalf("RemoveUnifiedTopicEntry failed: %v", err)
	}
	if bytes.Contains(filtered, []byte("oauthTokenInfoSentinelKey")) {
		t.Fatal("expected oauthTokenInfoSentinelKey to be removed")
	}
	if !bytes.Contains(filtered, []byte("anotherSentinelKey")) {
		t.Fatal("expected anotherSentinelKey to be retained")
	}
}
