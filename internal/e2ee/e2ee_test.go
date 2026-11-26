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
	"time"

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

func TestE2EE_Bin2Request_RawAndFlateAndMalformed(t *testing.T) {
	gwID, _ := GenerateIdentity()
	cliID, _ := GenerateIdentity()
	shared, _ := gwID.ECDH(cliID.PublicKeyB64())
	salt, _ := RandomBytes(16)
	devID := "dev_e2ee_bin2_req"
	devToken := "tok_bin2_req_123"
	devKey := DeriveDeviceKey(shared, salt, devID)

	var gotFileName string
	var gotBody []byte
	var gotContentLength int64

	innerMux := http.NewServeMux()
	innerMux.HandleFunc("/api/v1/attachments", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+devToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		gotFileName = r.Header.Get("X-File-Name")
		gotContentLength = r.ContentLength
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"att_bin2_ok"}`))
	})

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
	}

	outerMux := http.NewServeMux()
	outerMux.HandleFunc("/api/v1/e2ee/rpc", h.HandleRPC)
	outerSrv := httptest.NewServer(outerMux)
	defer outerSrv.Close()

	// 1. Binary v2 raw request (0x01) with arbitrary ZIP/binary bytes (zero base64 inflation)
	zipPayload := append([]byte{0x50, 0x4b, 0x03, 0x04, 0x00, 0xff, 0xfe, 0x80}, bytes.Repeat([]byte{0xab, 0xcd, 0xef, 0x01}, 4096)...)
	metaJSON, err := json.Marshal(InnerHTTPReqMeta{
		Method: "POST",
		Path:   "/api/v1/attachments",
		Headers: map[string]string{
			"Authorization": "Bearer " + devToken,
			"Content-Type":  "application/octet-stream",
			"X-File-Name":   "wechat_record.zip",
		},
	})
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}
	rawFrame := make([]byte, 1+4+len(metaJSON)+len(zipPayload))
	rawFrame[0] = FrameTypeBinRaw
	binary.BigEndian.PutUint32(rawFrame[1:5], uint32(len(metaJSON)))
	copy(rawFrame[5:5+len(metaJSON)], metaJSON)
	copy(rawFrame[5+len(metaJSON):], zipPayload)

	encReq, err := Encrypt(devKey, rawFrame, FrameAAD(devID, "c2s"))
	if err != nil {
		t.Fatalf("encrypt raw bin2 req: %v", err)
	}
	req, _ := http.NewRequest(http.MethodPost, outerSrv.URL+"/api/v1/e2ee/rpc", bytes.NewReader(encReq))
	req.Header.Set(HeaderDeviceID, devID)
	req.Header.Set(HeaderAcceptEnc, "bin2,flate")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do bin2 raw req: %v", err)
	}
	encResp, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, string(encResp))
	}
	for _, capTok := range SupportedCapsList() {
		if !headerContainsToken(resp.Header.Get(HeaderE2EECaps), capTok) {
			t.Fatalf("expected %s header containing %q, got %q", HeaderE2EECaps, capTok, resp.Header.Get(HeaderE2EECaps))
		}
	}
	if gotFileName != "wechat_record.zip" || gotContentLength != int64(len(zipPayload)) || !bytes.Equal(gotBody, zipPayload) {
		t.Fatalf("inner request mismatch: fileName=%q contentLen=%d bodyLen=%d want=%d", gotFileName, gotContentLength, len(gotBody), len(zipPayload))
	}

	// 2. Binary v2 flate-compressed request (0x02)
	compBody, ok := deflateRaw(rawFrame[1:])
	if !ok {
		t.Fatalf("expected repetitive payload to compress")
	}
	flateFrame := append([]byte{FrameTypeBinFlate}, compBody...)
	encFlate, err := Encrypt(devKey, flateFrame, FrameAAD(devID, "c2s"))
	if err != nil {
		t.Fatalf("encrypt flate bin2 req: %v", err)
	}
	gotBody = nil
	req2, _ := http.NewRequest(http.MethodPost, outerSrv.URL+"/api/v1/e2ee/rpc", bytes.NewReader(encFlate))
	req2.Header.Set(HeaderDeviceID, devID)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("do bin2 flate req: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK || !bytes.Equal(gotBody, zipPayload) {
		t.Fatalf("bin2 flate request failed: status=%d bodyMatch=%v", resp2.StatusCode, bytes.Equal(gotBody, zipPayload))
	}

	// 3. Malformed bin2 frames must fail cleanly with HTTP 400
	malformed := [][]byte{
		{FrameTypeBinRaw},
		{FrameTypeBinRaw, 0, 0, 0, 99, '{', '}'},
		{FrameTypeBinRaw, 0, 0, 0, 1, '{'},
		{FrameTypeBinRaw, 0, 0, 0, 4, 'n', 'o', 'p', 'e'},
		{FrameTypeBinFlate, 0xff, 0xff, 0xff},
	}
	for i, bad := range malformed {
		encBad, _ := Encrypt(devKey, bad, FrameAAD(devID, "c2s"))
		badReq, _ := http.NewRequest(http.MethodPost, outerSrv.URL+"/api/v1/e2ee/rpc", bytes.NewReader(encBad))
		badReq.Header.Set(HeaderDeviceID, devID)
		badResp, err := http.DefaultClient.Do(badReq)
		if err != nil {
			t.Fatalf("malformed[%d] do: %v", i, err)
		}
		badResp.Body.Close()
		if badResp.StatusCode != http.StatusBadRequest {
			t.Fatalf("malformed[%d]: expected 400, got %d", i, badResp.StatusCode)
		}
	}
}

func TestRelayGuardMiddleware_DisableWebOption(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	guarded := RelayGuardMiddlewareWithOptions(inner, RelayGuardOptions{DisableWeb: true})

	// Static Web/PWA assets must be blocked with 403 over relay when DisableWeb=true
	for _, path := range []string{"/", "/index.html", "/js/e2ee.js", "/sw.js", "/manifest.json"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req = req.WithContext(WithRelayTransport(req.Context()))
		rr := httptest.NewRecorder()
		guarded.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403 for disabled web path %s, got %d", path, rr.Code)
		}
	}

	// Core E2EE and healthz endpoints remain allowed
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/healthz"},
		{http.MethodPost, "/api/v1/e2ee/rpc"},
		{http.MethodPost, "/api/v1/e2ee/pair"},
		{http.MethodGet, "/api/v1/e2ee/ws"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req = req.WithContext(WithRelayTransport(req.Context()))
		rr := httptest.NewRecorder()
		guarded.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 for core path %s %s, got %d", tc.method, tc.path, rr.Code)
		}
	}

	// Plaintext API paths still return 426 so native apps auto-upgrade to E2EE
	apiReq := httptest.NewRequest(http.MethodGet, "/gateway/status", nil)
	apiReq = apiReq.WithContext(WithRelayTransport(apiReq.Context()))
	apiRR := httptest.NewRecorder()
	guarded.ServeHTTP(apiRR, apiReq)
	if apiRR.Code != http.StatusUpgradeRequired {
		t.Errorf("expected 426 for plaintext API, got %d", apiRR.Code)
	}

	// Non-relay LAN access to web shell remains unaffected
	lanReq := httptest.NewRequest(http.MethodGet, "/", nil)
	lanRR := httptest.NewRecorder()
	guarded.ServeHTTP(lanRR, lanReq)
	if lanRR.Code != http.StatusOK {
		t.Errorf("expected 200 for LAN web access, got %d", lanRR.Code)
	}
}

func TestE2EE_HTTP_ReplayProtection_And_ClockSkew(t *testing.T) {
	gwID, _ := GenerateIdentity()
	cliID, _ := GenerateIdentity()
	shared, _ := gwID.ECDH(cliID.PublicKeyB64())
	salt, _ := RandomBytes(16)
	devID := "dev_replay_test"
	devKey := DeriveDeviceKey(shared, salt, devID)

	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	calls := 0
	innerMux := http.NewServeMux()
	innerMux.HandleFunc("/mutate", func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	h := &Handler{
		Identity:     gwID,
		LookupKey:    func(id string) ([]byte, bool) { return devKey, id == devID },
		InnerHandler: innerMux,
		ReplayGuard:  NewReplayGuardWithOptions(5*time.Minute, 128, func() time.Time { return now }),
	}
	outerSrv := httptest.NewServer(http.HandlerFunc(h.HandleRPC))
	defer outerSrv.Close()

	sendFrame := func(encReq []byte) (*http.Response, []byte) {
		req, _ := http.NewRequest(http.MethodPost, outerSrv.URL+"/api/v1/e2ee/rpc", bytes.NewReader(encReq))
		req.Header.Set(HeaderDeviceID, devID)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("RPC do: %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, body
	}

	// 1. First request with ts + id succeeds
	inner1, _ := json.Marshal(InnerHTTPRequest{
		Method: "POST",
		Path:   "/mutate",
		TSMs:   now.UnixMilli(),
		ReqID:  "req_1234567890abcdef",
	})
	enc1, _ := Encrypt(devKey, inner1, FrameAAD(devID, "c2s"))
	resp1, _ := sendFrame(enc1)
	if resp1.StatusCode != http.StatusOK || calls != 1 {
		t.Fatalf("first request failed: status=%d calls=%d", resp1.StatusCode, calls)
	}

	// 2. Exact ciphertext replay MUST be rejected with 409 e2ee_replay_detected without hitting inner handler!
	respReplay, bodyReplay := sendFrame(enc1)
	if respReplay.StatusCode != http.StatusConflict || !strings.Contains(string(bodyReplay), ErrReplayDetected) {
		t.Fatalf("expected 409 %s on replay, got %d body=%s", ErrReplayDetected, respReplay.StatusCode, string(bodyReplay))
	}
	if calls != 1 {
		t.Fatalf("replayed request must not execute inner handler (calls=%d)", calls)
	}

	// 3. Skewed timestamp (>5m) MUST be rejected with 401 e2ee_replay_clock_skew + server_ts_ms
	innerSkew, _ := json.Marshal(InnerHTTPRequest{
		Method: "POST",
		Path:   "/mutate",
		TSMs:   now.Add(-10 * time.Minute).UnixMilli(),
		ReqID:  "req_skew_1234567890",
	})
	encSkew, _ := Encrypt(devKey, innerSkew, FrameAAD(devID, "c2s"))
	respSkew, bodySkew := sendFrame(encSkew)
	if respSkew.StatusCode != http.StatusUnauthorized || !strings.Contains(string(bodySkew), ErrReplayClockSkew) {
		t.Fatalf("expected 401 %s on skewed ts, got %d body=%s", ErrReplayClockSkew, respSkew.StatusCode, string(bodySkew))
	}
	var skewPayload struct {
		Error      string `json:"error"`
		ServerTSMs int64  `json:"server_ts_ms"`
	}
	if err := json.Unmarshal(bodySkew, &skewPayload); err != nil || skewPayload.ServerTSMs != now.UnixMilli() {
		t.Fatalf("expected server_ts_ms=%d, got %+v err=%v", now.UnixMilli(), skewPayload, err)
	}

	// 4. Legacy client without ts/id still accepted when StrictReplay=false, rejected when StrictReplay=true
	innerLegacy, _ := json.Marshal(InnerHTTPRequest{Method: "POST", Path: "/mutate"})
	encLegacy, _ := Encrypt(devKey, innerLegacy, FrameAAD(devID, "c2s"))
	respLegacy, _ := sendFrame(encLegacy)
	if respLegacy.StatusCode != http.StatusOK || calls != 2 {
		t.Fatalf("expected legacy request allowed when StrictReplay=false, got %d", respLegacy.StatusCode)
	}
	h.StrictReplay = true
	respStrict, bodyStrict := sendFrame(encLegacy)
	if respStrict.StatusCode != http.StatusBadRequest || !strings.Contains(string(bodyStrict), ErrReplayRequired) {
		t.Fatalf("expected 400 %s under StrictReplay=true, got %d body=%s", ErrReplayRequired, respStrict.StatusCode, string(bodyStrict))
	}
}

func TestE2EE_WS_MonotonicSeqAAD_BlocksReplayDropAndReorder(t *testing.T) {
	gwID, _ := GenerateIdentity()
	cliID, _ := GenerateIdentity()
	shared, _ := gwID.ECDH(cliID.PublicKeyB64())
	salt, _ := RandomBytes(16)
	devID := "dev_ws_seq"
	devToken := "tok_ws_seq_123456"
	devKey := DeriveDeviceKey(shared, salt, devID)

	wsUpgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	innerMux := http.NewServeMux()
	innerMux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			mt, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			_ = conn.WriteMessage(mt, append([]byte("ack:"), msg...))
		}
	})
	localSrv := httptest.NewServer(innerMux)
	defer localSrv.Close()
	localAddr := strings.TrimPrefix(localSrv.URL, "http://")

	h := &Handler{
		Identity:      gwID,
		LookupKey:     func(id string) ([]byte, bool) { return devKey, id == devID },
		ValidateToken: func(raw string) (string, bool) { return devID, raw == devToken },
		LocalWSAddr:   func() string { return localAddr },
	}
	outerSrv := httptest.NewServer(http.HandlerFunc(h.HandleWS))
	defer outerSrv.Close()

	wsURL := "ws" + strings.TrimPrefix(outerSrv.URL, "http") + "/api/v1/e2ee/ws?d=" + devID
	wsConn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer wsConn.Close()
	if !headerContainsToken(resp.Header.Get(HeaderE2EECaps), CapWSSeq1) {
		t.Fatalf("expected WS upgrade header %s containing %s, got %q", HeaderE2EECaps, CapWSSeq1, resp.Header.Get(HeaderE2EECaps))
	}

	// Send Frame 0 with seq="v1" + replay ts/id
	openJSON, _ := json.Marshal(WSOpenFrame{
		Path:    "/echo",
		Headers: map[string]string{"Authorization": "Bearer " + devToken},
		Seq:     "v1",
		TSMs:    time.Now().UnixMilli(),
		ReqID:   "ws_open_1234567890",
	})
	encOpen, _ := Encrypt(devKey, openJSON, FrameAAD(devID, "ws-open"))
	if err := wsConn.WriteMessage(websocket.BinaryMessage, encOpen); err != nil {
		t.Fatalf("write open: %v", err)
	}

	// Frame seq=0 & seq=1 succeed in order
	enc0, _ := Encrypt(devKey, append([]byte{byte(websocket.TextMessage)}, []byte("m0")...), FrameSeqAAD(devID, "ws-c2s", 0))
	enc1, _ := Encrypt(devKey, append([]byte{byte(websocket.TextMessage)}, []byte("m1")...), FrameSeqAAD(devID, "ws-c2s", 1))
	_ = wsConn.WriteMessage(websocket.BinaryMessage, enc0)
	_ = wsConn.WriteMessage(websocket.BinaryMessage, enc1)

	for wantSeq, wantMsg := range []string{"ack:m0", "ack:m1"} {
		_, encAck, err := wsConn.ReadMessage()
		if err != nil {
			t.Fatalf("read ack %d: %v", wantSeq, err)
		}
		plain, err := Decrypt(devKey, encAck, FrameSeqAAD(devID, "ws-s2c", uint64(wantSeq)))
		if err != nil || string(plain[1:]) != wantMsg {
			t.Fatalf("ack %d mismatch: err=%v got=%q want=%q", wantSeq, err, string(plain), wantMsg)
		}
	}

	// Replaying enc1 (which carries seq=1 while server now expects seq=2) MUST break GCM and close the WS!
	_ = wsConn.WriteMessage(websocket.BinaryMessage, enc1)
	_ = wsConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := wsConn.ReadMessage(); err == nil {
		t.Fatalf("expected WS connection to close when replaying previous frame seq=1")
	}

	// Replaying the exact same Frame 0 on a new WS connection MUST be rejected by Frame 0 replay guard!
	wsConn2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial2: %v", err)
	}
	defer wsConn2.Close()
	_ = wsConn2.WriteMessage(websocket.BinaryMessage, encOpen)
	_ = wsConn2.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := wsConn2.ReadMessage(); err == nil {
		t.Fatalf("expected replayed WS Frame 0 to be rejected")
	}
}
