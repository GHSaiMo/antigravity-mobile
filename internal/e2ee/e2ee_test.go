package e2ee

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"encoding/binary"
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
	if httpResp.ContentLength != int64(len(encResp)) {
		t.Fatalf("expected outer Content-Length=%d, got %d", len(encResp), httpResp.ContentLength)
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

func TestE2EE_Bin2Flate_HTTP_And_WebSocket(t *testing.T) {
	gwID, _ := GenerateIdentity()
	cliID, _ := GenerateIdentity()
	shared, _ := gwID.ECDH(cliID.PublicKeyB64())
	salt, _ := RandomBytes(16)
	devID := "dev_e2ee_flate"
	devToken := "tok_flate_123456"
	devKey := DeriveDeviceKey(shared, salt, devID)

	bigBody := strings.Repeat(`{"cascadeId":"abc12345","status":"RUNNING","title":"Multigravity Benchmark Session"},`, 120)
	tinyBody := `{"ok":true}`
	wsUpgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	innerMux := http.NewServeMux()
	innerMux.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Antigravity-Cloud-URL", "https://r-a1b2c3d4.jp1.jiuge.space")
		_, _ = w.Write([]byte(bigBody))
	})
	innerMux.HandleFunc("/tiny", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(tinyBody))
	})
	innerMux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.TextMessage, []byte(tinyBody))
		_ = conn.WriteMessage(websocket.TextMessage, []byte(bigBody))
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

	// 1. Large HTTP RPC with X-Mgy-E2EE-Enc: bin2,flate -> FrameTypeBinFlate (0x02) & >5x smaller than raw
	doRPC := func(path string) []byte {
		innerReqBytes, _ := json.Marshal(InnerHTTPRequest{
			Method:  "GET",
			Path:    path,
			Headers: map[string]string{"Authorization": "Bearer " + devToken},
		})
		encReq, _ := Encrypt(devKey, innerReqBytes, FrameAAD(devID, "c2s"))
		req, _ := http.NewRequest(http.MethodPost, outerSrv.URL+"/api/v1/e2ee/rpc", bytes.NewReader(encReq))
		req.Header.Set(HeaderDeviceID, devID)
		req.Header.Set(HeaderAcceptEnc, "bin2,flate")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("RPC %s: %v", path, err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("RPC %s status=%d", path, resp.StatusCode)
		}
		plain, err := Decrypt(devKey, raw, FrameAAD(devID, "s2c"))
		if err != nil {
			t.Fatalf("decrypt %s: %v", path, err)
		}
		return plain
	}

	plainBig := doRPC("/big")
	if len(plainBig) < 2 || plainBig[0] != FrameTypeBinFlate {
		t.Fatalf("expected FrameTypeBinFlate (0x02), got first byte 0x%02x (len=%d)", plainBig[0], len(plainBig))
	}
	if len(plainBig) >= len(bigBody)/5 {
		t.Fatalf("expected >=5x compression on repetitive JSON, got compressed=%d vs raw=%d", len(plainBig), len(bigBody))
	}
	fr := flate.NewReader(bytes.NewReader(plainBig[1:]))
	unpacked, err := io.ReadAll(fr)
	_ = fr.Close()
	if err != nil {
		t.Fatalf("inflate big RPC: %v", err)
	}
	metaLen := binary.BigEndian.Uint32(unpacked[:4])
	var meta InnerHTTPMeta
	if err := json.Unmarshal(unpacked[4:4+metaLen], &meta); err != nil {
		t.Fatalf("unmarshal bin2 meta: %v", err)
	}
	bodyGot := string(unpacked[4+metaLen:])
	if meta.Status != 200 || bodyGot != bigBody {
		t.Fatalf("bin2 flate mismatch: status=%d bodyLen=%d want=%d", meta.Status, len(bodyGot), len(bigBody))
	}
	if meta.Headers[http.CanonicalHeaderKey("X-Antigravity-Cloud-URL")] != "https://r-a1b2c3d4.jp1.jiuge.space" {
		t.Fatalf("missing cloud URL in bin2 meta: %+v", meta.Headers)
	}

	// 2. Tiny HTTP RPC -> FrameTypeBinRaw (0x01)
	plainTiny := doRPC("/tiny")
	if len(plainTiny) < 5 || plainTiny[0] != FrameTypeBinRaw {
		t.Fatalf("expected FrameTypeBinRaw (0x01), got 0x%02x", plainTiny[0])
	}
	tMetaLen := binary.BigEndian.Uint32(plainTiny[1:5])
	if got := string(plainTiny[5+tMetaLen:]); got != tinyBody {
		t.Fatalf("tiny bin2 mismatch: got=%q want=%q", got, tinyBody)
	}

	// 3. WebSocket with enc="flate" -> small frame uncompressed (0x01), big frame compressed (0x81)
	wsURL := "ws" + strings.TrimPrefix(outerSrv.URL, "http") + "/api/v1/e2ee/ws?d=" + devID
	wsConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer wsConn.Close()

	openJSON, _ := json.Marshal(WSOpenFrame{
		Path:    "/ws",
		Headers: map[string]string{"Authorization": "Bearer " + devToken},
		Enc:     "flate",
	})
	encOpen, _ := Encrypt(devKey, openJSON, FrameAAD(devID, "ws-open"))
	if err := wsConn.WriteMessage(websocket.BinaryMessage, encOpen); err != nil {
		t.Fatalf("write ws open: %v", err)
	}

	_, encFrame1, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("read ws frame1: %v", err)
	}
	p1, err := Decrypt(devKey, encFrame1, FrameAAD(devID, "ws-s2c"))
	if err != nil || p1[0] != byte(websocket.TextMessage) || string(p1[1:]) != tinyBody {
		t.Fatalf("unexpected tiny ws frame: err=%v hdr=0x%02x body=%q", err, p1[0], string(p1[1:]))
	}

	_, encFrame2, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("read ws frame2: %v", err)
	}
	p2, err := Decrypt(devKey, encFrame2, FrameAAD(devID, "ws-s2c"))
	if err != nil || p2[0] != (byte(websocket.TextMessage)|wsCompressedFlag) {
		t.Fatalf("expected compressed ws frame 0x81, got err=%v hdr=0x%02x", err, p2[0])
	}
	wsFr := flate.NewReader(bytes.NewReader(p2[1:]))
	wsUnpacked, err := io.ReadAll(wsFr)
	_ = wsFr.Close()
	if err != nil || string(wsUnpacked) != bigBody {
		t.Fatalf("ws inflated mismatch: err=%v len=%d want=%d", err, len(wsUnpacked), len(bigBody))
	}
}
