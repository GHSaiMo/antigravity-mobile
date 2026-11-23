package e2ee

import (
	"bytes"
	"compress/flate"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

const (
	// HeaderDeviceID identifies which paired device's E2EE session key decrypts the request.
	HeaderDeviceID = "X-Mgy-E2EE-Device"
	// HeaderAcceptEnc advertises client-supported E2EE inner response framing/compression (e.g. "bin2,flate").
	HeaderAcceptEnc = "X-Mgy-E2EE-Enc"

	// Inner binary v2 response framing markers (never collide with legacy JSON '{' = 0x7b).
	FrameTypeBinRaw   byte = 0x01 // [0x01][u32 BE metaLen][metaJSON][rawBody]
	FrameTypeBinFlate byte = 0x02 // [0x02][raw-deflate([u32 BE metaLen][metaJSON][rawBody])]

	compressMinBytes  = 512
	flateLevel        = 4 // fast speed + strong JSON compression (~4.5x-8x smaller than base64 JSON)
	wsCompressedFlag  = byte(0x80)

	maxEncryptedRPCBytes = 64 << 20 // 64 MiB safety ceiling matching gateway MaxBytesMiddleware
)

var flateWriterPool = sync.Pool{
	New: func() any {
		w, _ := flate.NewWriter(io.Discard, flateLevel)
		return w
	},
}

// deflateRaw compresses src with RFC 1951 raw deflate (compatible with Go flate,
// iOS NSData.decompressed(using: .zlib), Android Inflater(true), and Web DecompressionStream("deflate-raw")).
func deflateRaw(src []byte) ([]byte, bool) {
	if len(src) < compressMinBytes {
		return nil, false
	}
	var buf bytes.Buffer
	buf.Grow(len(src) / 3)
	fw := flateWriterPool.Get().(*flate.Writer)
	fw.Reset(&buf)
	_, wErr := fw.Write(src)
	cErr := fw.Close()
	fw.Reset(io.Discard)
	flateWriterPool.Put(fw)
	if wErr != nil || cErr != nil {
		return nil, false
	}
	out := buf.Bytes()
	if len(out) >= len(src) {
		return nil, false
	}
	return out, true
}

func headerContainsToken(raw, token string) bool {
	if raw == "" {
		return false
	}
	for _, part := range strings.Split(raw, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}

// InnerHTTPMeta is the JSON metadata header inside Binary v2 E2EE responses (without base64 body).
type InnerHTTPMeta struct {
	Status  int               `json:"s"`
	Headers map[string]string `json:"h,omitempty"`
}

func encodeBinaryHTTPResponse(status int, headers map[string]string, body []byte, allowFlate bool) ([]byte, error) {
	metaJSON, err := json.Marshal(InnerHTTPMeta{
		Status:  status,
		Headers: headers,
	})
	if err != nil {
		return nil, err
	}
	rawLen := 4 + len(metaJSON) + len(body)
	payload := make([]byte, 1+rawLen)
	payload[0] = FrameTypeBinRaw
	binary.BigEndian.PutUint32(payload[1:5], uint32(len(metaJSON)))
	copy(payload[5:5+len(metaJSON)], metaJSON)
	copy(payload[5+len(metaJSON):], body)

	if allowFlate && rawLen >= compressMinBytes {
		if comp, ok := deflateRaw(payload[1:]); ok && len(comp) < rawLen {
			out := make([]byte, 1+len(comp))
			out[0] = FrameTypeBinFlate
			copy(out[1:], comp)
			return out, nil
		}
	}
	return payload, nil
}

// InnerHTTPRequest is the JSON-encoded plaintext inside `POST /api/v1/e2ee/rpc`.
type InnerHTTPRequest struct {
	Method  string            `json:"m"`
	Path    string            `json:"p"`
	Headers map[string]string `json:"h,omitempty"`
	BodyB64 string            `json:"b,omitempty"`
}

// InnerHTTPResponse is the JSON-encoded plaintext returned inside `POST /api/v1/e2ee/rpc`.
type InnerHTTPResponse struct {
	Status  int               `json:"s"`
	Headers map[string]string `json:"h,omitempty"`
	BodyB64 string            `json:"b,omitempty"`
}

// KeyLookup resolves a paired device's 32-byte AES-256-GCM key by deviceID.
type KeyLookup func(deviceID string) ([]byte, bool)

// TokenValidator validates a raw device bearer token and returns the owning deviceID.
type TokenValidator func(rawToken string) (string, bool)

// Handler serves E2EE-encrypted HTTP RPC and WebSocket tunnels.
type Handler struct {
	Identity       *Identity
	LookupKey      KeyLookup
	ValidateToken  TokenValidator
	InnerHandler   http.Handler
	LocalWSAddr    func() string
	PairDispatcher func(rawPairBody []byte, clientPubB64 string, outerReq *http.Request) (pairRespJSON []byte, deviceID string, status int, err error)
}

// RelayGuardMiddleware strictly enforces zero-knowledge E2EE on any connection arriving via relay_v1.
// Plaintext API/WebSocket calls on LAN, Tailscale, or legacy Cloudflare Tunnel are completely unaffected.
func RelayGuardMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsRelayTransport(r.Context()) {
			next.ServeHTTP(w, r)
			return
		}
		if isRelayAllowedPath(r.Method, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusUpgradeRequired)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "e2ee_required",
			"message": "Plaintext API traffic is blocked on relay transport; use E2EE channel.",
		})
	})
}

func isRelayAllowedPath(method, path string) bool {
	switch path {
	case "/healthz":
		return method == http.MethodGet || method == http.MethodHead
	case "/api/v1/e2ee/rpc", "/api/v1/e2ee/pair":
		return method == http.MethodPost || method == http.MethodOptions
	case "/api/v1/e2ee/ws":
		return method == http.MethodGet
	}

	// Allow read-only static PWA shell assets so browsers can bootstrap Web Crypto E2EE
	if method == http.MethodGet || method == http.MethodHead {
		switch path {
		case "/", "/index.html", "/style.css", "/sw.js", "/favicon.ico",
			"/manifest.json", "/manifest-en.json", "/manifest-ja.json", "/mermaid.min.js":
			return true
		}
		if strings.HasPrefix(path, "/js/") || strings.HasPrefix(path, "/icons/") {
			return true
		}
	}
	return false
}

// HandleRPC decrypts an encrypted HTTP request envelope, runs it through InnerHandler, and encrypts the response.
func (h *Handler) HandleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeE2EEError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	deviceID := strings.TrimSpace(r.Header.Get(HeaderDeviceID))
	if deviceID == "" {
		writeE2EEError(w, http.StatusUnauthorized, "missing_e2ee_device")
		return
	}
	key, ok := h.LookupKey(deviceID)
	if !ok || len(key) != KeySize {
		writeE2EEError(w, http.StatusUnauthorized, "unknown_e2ee_device")
		return
	}

	encBody, err := io.ReadAll(io.LimitReader(r.Body, maxEncryptedRPCBytes+1))
	if err != nil {
		writeE2EEError(w, http.StatusBadRequest, "read_body_failed")
		return
	}
	if len(encBody) > maxEncryptedRPCBytes {
		writeE2EEError(w, http.StatusRequestEntityTooLarge, "payload_too_large")
		return
	}

	plainBytes, err := Decrypt(key, encBody, FrameAAD(deviceID, "c2s"))
	if err != nil {
		writeE2EEError(w, http.StatusUnauthorized, "e2ee_decrypt_failed")
		return
	}

	var inner InnerHTTPRequest
	if err := json.Unmarshal(plainBytes, &inner); err != nil {
		writeE2EEError(w, http.StatusBadRequest, "invalid_inner_request")
		return
	}
	if !strings.HasPrefix(inner.Path, "/") || strings.HasPrefix(inner.Path, "//") || strings.HasPrefix(inner.Path, "/api/v1/e2ee/") {
		writeE2EEError(w, http.StatusBadRequest, "invalid_inner_path")
		return
	}
	if inner.Method == "" {
		inner.Method = http.MethodGet
	}

	var innerBody []byte
	if inner.BodyB64 != "" {
		innerBody, err = base64.StdEncoding.DecodeString(inner.BodyB64)
		if err != nil {
			writeE2EEError(w, http.StatusBadRequest, "invalid_inner_body_b64")
			return
		}
	}

	// Build inner request WITHOUT relayTransportKey so inner middleware executes normally,
	// while preserving external Host / RemoteAddr / CF-Connecting-IP for strict auth & rate-limit checks.
	innerCtx := context.WithoutCancel(r.Context())
	innerReq, err := http.NewRequestWithContext(innerCtx, strings.ToUpper(inner.Method), "http://"+r.Host+inner.Path, bytes.NewReader(innerBody))
	if err != nil {
		writeE2EEError(w, http.StatusBadRequest, "build_inner_request_failed")
		return
	}
	innerReq.Host = r.Host
	innerReq.RemoteAddr = r.RemoteAddr
	for k, v := range inner.Headers {
		// Prevent inner GzipHandler from emitting pre-compressed bytes that synthetic
		// client-side HTTP responses (URLSession / OkHttp / fetch) would not auto-inflate.
		if strings.EqualFold(strings.TrimSpace(k), "Accept-Encoding") {
			continue
		}
		innerReq.Header.Set(k, v)
	}
	innerReq.Header.Del("Accept-Encoding")
	if cfIP := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); cfIP != "" {
		innerReq.Header.Set("CF-Connecting-IP", cfIP)
	}
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		innerReq.Header.Set("X-Forwarded-For", xff)
	}
	innerReq.Header.Set("X-Forwarded-Proto", "https")

	rec := newBufferedResponseWriter()
	h.InnerHandler.ServeHTTP(rec, innerReq)

	respHeaders := make(map[string]string, len(rec.header))
	for k, vals := range rec.header {
		if len(vals) == 0 {
			continue
		}
		respHeaders[k] = strings.Join(vals, ", ")
	}
	acceptEnc := r.Header.Get(HeaderAcceptEnc)
	var respPlain []byte
	if headerContainsToken(acceptEnc, "bin2") {
		respPlain, err = encodeBinaryHTTPResponse(
			rec.statusCode,
			respHeaders,
			rec.body.Bytes(),
			headerContainsToken(acceptEnc, "flate"),
		)
	} else {
		innerResp := InnerHTTPResponse{
			Status:  rec.statusCode,
			Headers: respHeaders,
		}
		if rec.body.Len() > 0 {
			innerResp.BodyB64 = base64.StdEncoding.EncodeToString(rec.body.Bytes())
		}
		respPlain, err = json.Marshal(innerResp)
	}
	if err != nil {
		writeE2EEError(w, http.StatusInternalServerError, "encode_inner_response_failed")
		return
	}
	respEnc, err := Encrypt(key, respPlain, FrameAAD(deviceID, "s2c"))
	if err != nil {
		writeE2EEError(w, http.StatusInternalServerError, "encrypt_response_failed")
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(respEnc)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(respEnc)
}

// EncryptedPairEnvelope is the outer JSON envelope for `POST /api/v1/e2ee/pair`.
type EncryptedPairEnvelope struct {
	ClientPub  string `json:"client_pub"`
	Ciphertext string `json:"ciphertext"` // Base64 standard or URL encoding of AES-256-GCM frame
}

// HandleEncryptedPair processes an end-to-end encrypted initial pairing request over relay_v1
// using the gateway's P-256 public key embedded in the QR code (`&pk=...`).
func (h *Handler) HandleEncryptedPair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeE2EEError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	if h.Identity == nil || h.PairDispatcher == nil {
		writeE2EEError(w, http.StatusServiceUnavailable, "e2ee_not_configured")
		return
	}
	var env EncryptedPairEnvelope
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&env); err != nil {
		writeE2EEError(w, http.StatusBadRequest, "invalid_pair_envelope")
		return
	}
	clientPub := strings.TrimSpace(env.ClientPub)
	if clientPub == "" || strings.TrimSpace(env.Ciphertext) == "" {
		writeE2EEError(w, http.StatusBadRequest, "missing_pair_fields")
		return
	}
	shared, err := h.Identity.ECDH(clientPub)
	if err != nil {
		writeE2EEError(w, http.StatusBadRequest, "invalid_client_pubkey")
		return
	}
	pairKey := DerivePairKey(shared, clientPub, h.Identity.PublicKeyB64())
	encFrame, err := DecodeBase64Flexible(env.Ciphertext)
	if err != nil {
		writeE2EEError(w, http.StatusBadRequest, "invalid_ciphertext_encoding")
		return
	}
	plainReq, err := Decrypt(pairKey, encFrame, PairFrameAAD(clientPub, "c2s"))
	if err != nil {
		writeE2EEError(w, http.StatusUnauthorized, "pair_decrypt_failed")
		return
	}

	respPlain, _, status, err := h.PairDispatcher(plainReq, clientPub, r)
	if err != nil {
		writeE2EEError(w, http.StatusInternalServerError, "pair_dispatch_failed")
		return
	}
	if status == 0 {
		status = http.StatusOK
	}

	wrappedResp, _ := json.Marshal(map[string]any{
		"status": status,
		"body":   json.RawMessage(respPlain),
	})
	respEnc, err := Encrypt(pairKey, wrappedResp, PairFrameAAD(clientPub, "s2c"))
	if err != nil {
		writeE2EEError(w, http.StatusInternalServerError, "pair_encrypt_failed")
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"ciphertext": base64.StdEncoding.EncodeToString(respEnc),
	})
}

func writeE2EEError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}

type bufferedResponseWriter struct {
	header      http.Header
	body        bytes.Buffer
	statusCode  int
	wroteHeader bool
}

func newBufferedResponseWriter() *bufferedResponseWriter {
	return &bufferedResponseWriter{
		header:     make(http.Header),
		statusCode: http.StatusOK,
	}
}

func (b *bufferedResponseWriter) Header() http.Header { return b.header }

func (b *bufferedResponseWriter) WriteHeader(code int) {
	if !b.wroteHeader {
		b.statusCode = code
		b.wroteHeader = true
	}
}

func (b *bufferedResponseWriter) Write(p []byte) (int, error) {
	if !b.wroteHeader {
		b.WriteHeader(http.StatusOK)
	}
	return b.body.Write(p)
}

func (b *bufferedResponseWriter) Flush() {}
