package e2ee

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestECDH_HKDF_AESGCM_RoundTripAndTamperRejection(t *testing.T) {
	gw, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("gen gw: %v", err)
	}
	cli, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("gen cli: %v", err)
	}

	s1, err := gw.ECDH(cli.PublicKeyB64())
	if err != nil {
		t.Fatalf("gw ECDH: %v", err)
	}
	s2, err := cli.ECDH(gw.PublicKeyB64())
	if err != nil {
		t.Fatalf("cli ECDH: %v", err)
	}
	if !bytes.Equal(s1, s2) {
		t.Fatalf("ECDH shared secrets diverged")
	}

	salt, _ := RandomBytes(16)
	k1 := DeriveDeviceKey(s1, salt, "dev_123")
	k2 := DeriveDeviceKey(s2, salt, "dev_123")
	if !bytes.Equal(k1, k2) || len(k1) != KeySize {
		t.Fatalf("derived device keys mismatch")
	}

	aad := FrameAAD("dev_123", "c2s")
	enc, err := Encrypt(k1, []byte("secret-code-and-prompt"), aad)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	dec, err := Decrypt(k2, enc, aad)
	if err != nil || string(dec) != "secret-code-and-prompt" {
		t.Fatalf("Decrypt failed: err=%v dec=%q", err, string(dec))
	}

	// Wrong direction AAD must be rejected (anti-reflection)
	if _, err := Decrypt(k2, enc, FrameAAD("dev_123", "s2c")); err == nil {
		t.Fatalf("expected direction AAD mismatch to fail")
	}
	// Bit-flipped ciphertext must be rejected
	enc[len(enc)-1] ^= 0x01
	if _, err := Decrypt(k2, enc, aad); err == nil {
		t.Fatalf("expected tampered ciphertext to fail")
	}
}

func TestRelayGuardMiddleware_BlocksPlaintextAPIsOnRelay(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("inner-ok"))
	})
	guarded := RelayGuardMiddleware(inner)

	// 1. Non-relay (LAN/CF) request to /gateway/models -> allowed
	reqLAN := httptest.NewRequest(http.MethodGet, "/gateway/models", nil)
	rrLAN := httptest.NewRecorder()
	guarded.ServeHTTP(rrLAN, reqLAN)
	if rrLAN.Code != http.StatusOK {
		t.Fatalf("expected LAN request allowed, got %d", rrLAN.Code)
	}

	// 2. Relay request to plaintext /gateway/models -> 426 Upgrade Required!
	reqRelayPlain := httptest.NewRequest(http.MethodGet, "/gateway/models", nil)
	reqRelayPlain = reqRelayPlain.WithContext(WithRelayTransport(reqRelayPlain.Context()))
	rrRelayPlain := httptest.NewRecorder()
	guarded.ServeHTTP(rrRelayPlain, reqRelayPlain)
	if rrRelayPlain.Code != http.StatusUpgradeRequired {
		t.Fatalf("expected 426 for plaintext API over relay, got %d", rrRelayPlain.Code)
	}

	// 3. Relay request to /api/v1/e2ee/rpc, /healthz, and static PWA shell -> allowed
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/healthz"},
		{http.MethodPost, "/api/v1/e2ee/rpc"},
		{http.MethodPost, "/api/v1/e2ee/pair"},
		{http.MethodGet, "/api/v1/e2ee/ws"},
		{http.MethodGet, "/"},
		{http.MethodGet, "/js/e2ee.js"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req = req.WithContext(WithRelayTransport(req.Context()))
		rr := httptest.NewRecorder()
		guarded.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected relay %s %s allowed, got %d", tc.method, tc.path, rr.Code)
		}
	}
}

func TestE2EE_HTTP_RPC_And_WebSocket_EndToEnd(t *testing.T) {
	gwID, _ := GenerateIdentity()
	cliID, _ := GenerateIdentity()
	shared, _ := gwID.ECDH(cliID.PublicKeyB64())
	salt, _ := RandomBytes(16)
	devID := "dev_e2ee_test"
	devToken := "tok_secret_987654"
	devKey := DeriveDeviceKey(shared, salt, devID)

	wsUpgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	innerMux := http.NewServeMux()
	innerMux.HandleFunc("/gateway/secret", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+devToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("X-Antigravity-Cloud-URL", "https://r-a1b2c3d4.jiuge.space")
		_, _ = w.Write([]byte("top-secret-payload:" + r.URL.Query().Get("q")))
	})
	innerMux.HandleFunc("/gateway/events", func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"hello","rev":42}`))
		mt, msg, err := conn.ReadMessage()
		if err == nil {
			_ = conn.WriteMessage(mt, append([]byte("ack:"), msg...))
		}
	})

	localSrv := httptest.NewServer(innerMux)
	defer localSrv.Close()
	localAddr := strings.TrimPrefix(localSrv.URL, "http://")

	h := &Handler{
		Identity: gwID,
		LookupKey: func(id string) ([]byte, bool) {
			if id == devID {
				return devKey, true
			}
			return nil, false
		},
		ValidateToken: func(raw string) (string, bool) {
			if raw == devToken {
				return devID, true
			}
			return "", false
		},
		InnerHandler: innerMux,
		LocalWSAddr:  func() string { return localAddr },
	}

	outerMux := http.NewServeMux()
	outerMux.HandleFunc("/api/v1/e2ee/rpc", h.HandleRPC)
	outerMux.HandleFunc("/api/v1/e2ee/ws", h.HandleWS)
	outerSrv := httptest.NewServer(outerMux)
	defer outerSrv.Close()

	// 1. Test encrypted HTTP RPC
	innerReqBytes, _ := json.Marshal(InnerHTTPRequest{
		Method:  "GET",
		Path:    "/gateway/secret?q=hello",
		Headers: map[string]string{"Authorization": "Bearer " + devToken},
	})
	encReq, err := Encrypt(devKey, innerReqBytes, FrameAAD(devID, "c2s"))
	if err != nil {
		t.Fatalf("encrypt req: %v", err)
	}
	httpReq, _ := http.NewRequest(http.MethodPost, outerSrv.URL+"/api/v1/e2ee/rpc", bytes.NewReader(encReq))
	httpReq.Header.Set(HeaderDeviceID, devID)
	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("RPC do: %v", err)
	}
	encResp, _ := io.ReadAll(httpResp.Body)
	httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		t.Fatalf("expected outer 200, got %d", httpResp.StatusCode)
	}
	plainResp, err := Decrypt(devKey, encResp, FrameAAD(devID, "s2c"))
	if err != nil {
		t.Fatalf("decrypt resp: %v", err)
	}
	var decoded InnerHTTPResponse
	if err := json.Unmarshal(plainResp, &decoded); err != nil {
		t.Fatalf("unmarshal inner resp: %v", err)
	}
	bodyBytes, _ := base64.StdEncoding.DecodeString(decoded.BodyB64)
	if decoded.Status != 200 || string(bodyBytes) != "top-secret-payload:hello" {
		t.Fatalf("unexpected inner response: status=%d body=%q", decoded.Status, string(bodyBytes))
	}
	if decoded.Headers[http.CanonicalHeaderKey("X-Antigravity-Cloud-URL")] != "https://r-a1b2c3d4.jiuge.space" {
		t.Fatalf("missing inner cloud url header: %+v", decoded.Headers)
	}

	// 2. Test encrypted WebSocket tunnel
	wsURL := "ws" + strings.TrimPrefix(outerSrv.URL, "http") + "/api/v1/e2ee/ws?d=" + devID
	wsConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer wsConn.Close()

	openJSON, _ := json.Marshal(WSOpenFrame{
		Path:    "/gateway/events",
		Headers: map[string]string{"Authorization": "Bearer " + devToken},
	})
	encOpen, _ := Encrypt(devKey, openJSON, FrameAAD(devID, "ws-open"))
	if err := wsConn.WriteMessage(websocket.BinaryMessage, encOpen); err != nil {
		t.Fatalf("write open: %v", err)
	}

	// Read encrypted hello frame
	mt, encHello, err := wsConn.ReadMessage()
	if err != nil || mt != websocket.BinaryMessage {
		t.Fatalf("read hello: err=%v mt=%d", err, mt)
	}
	plainHello, err := Decrypt(devKey, encHello, FrameAAD(devID, "ws-s2c"))
	if err != nil || plainHello[0] != byte(websocket.TextMessage) || string(plainHello[1:]) != `{"type":"hello","rev":42}` {
		t.Fatalf("unexpected decrypted hello: err=%v plain=%q", err, string(plainHello))
	}

	// Send encrypted text message -> receive encrypted ack
	msgPlain := append([]byte{byte(websocket.TextMessage)}, []byte("ping-e2ee")...)
	encMsg, _ := Encrypt(devKey, msgPlain, FrameAAD(devID, "ws-c2s"))
	if err := wsConn.WriteMessage(websocket.BinaryMessage, encMsg); err != nil {
		t.Fatalf("write encrypted msg: %v", err)
	}
	_, encAck, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("read ack: %v", err)
	}
	plainAck, err := Decrypt(devKey, encAck, FrameAAD(devID, "ws-s2c"))
	if err != nil || string(plainAck[1:]) != "ack:ping-e2ee" {
		t.Fatalf("unexpected decrypted ack: err=%v plain=%q", err, string(plainAck))
	}
}
