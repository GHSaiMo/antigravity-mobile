package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"antigravity-mobile/internal/proxy"
)

// nodeTransport builds a tuned HTTP transport for fast LAN node communication.
func newNodeTransport() *http.Transport {
	return &http.Transport{
		Proxy: nil, // LAN node-to-node traffic must NEVER traverse system HTTP proxies
		DialContext: (&net.Dialer{
			Timeout:   800 * time.Millisecond,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          128,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   2 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

func (h *Hub) applyNodeHeaders(dst http.Header, src http.Header, node NodeConfig) {
	if src != nil {
		for k, vals := range src {
			if isHopByHopHeader(k) ||
				strings.EqualFold(k, "Authorization") ||
				strings.EqualFold(k, "Cookie") ||
				strings.EqualFold(k, "Host") ||
				strings.EqualFold(k, "Accept-Encoding") ||
				strings.EqualFold(k, HeaderHubForwarded) ||
				strings.EqualFold(k, HeaderHubToken) {
				continue
			}
			for _, v := range vals {
				dst.Add(k, v)
			}
		}
	}
	dst.Set(HeaderHubForwarded, "1")
	if node.Token != "" {
		dst.Set(HeaderHubToken, node.Token)
		dst.Set("Authorization", "Bearer "+node.Token)
	}
	if adv := NormalizeBaseURL(h.opts.AdvertiseURL); adv != "" {
		dst.Set(HeaderHubURL, adv)
	}
}

func isHopByHopHeader(k string) bool {
	switch strings.ToLower(strings.TrimSpace(k)) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
		"te", "trailer", "transfer-encoding", "upgrade", "proxy-connection":
		return true
	default:
		return false
	}
}

// buildNodeURL constructs the full target URL for a node given path and raw query.
func buildNodeURL(baseURL, reqPath, rawQuery string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	u.Path = strings.TrimRight(u.Path, "/") + reqPath
	u.RawQuery = rawQuery
	return u.String(), nil
}

// doNodeJSON performs a short/bounded HTTP request to a remote node and returns status code + body.
func (h *Hub) doNodeRequest(ctx context.Context, client *http.Client, node NodeConfig, method, reqPath, rawQuery string, body []byte, contentType string) (int, http.Header, []byte, error) {
	targetURL, err := buildNodeURL(node.BaseURL, reqPath, rawQuery)
	if err != nil {
		return 0, nil, nil, err
	}
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, targetURL, bodyReader)
	if err != nil {
		return 0, nil, nil, err
	}
	h.applyNodeHeaders(req.Header, nil, node)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if strings.HasPrefix(reqPath, "/api/exa.") || strings.HasPrefix(reqPath, "/exa.") {
		req.Header.Set("Connect-Protocol-Version", "1")
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return resp.StatusCode, resp.Header, nil, err
	}
	return resp.StatusCode, resp.Header, respBody, nil
}

// forwardHTTPToNode transparently reverse-proxies an HTTP request (including streaming responses)
// to the target remote node, optionally overriding path, rawQuery, or body bytes.
// If inspectResp is non-nil and the response is JSON/small (non-streaming), the body is passed to inspectResp before returning.
func (h *Hub) forwardHTTPToNode(
	w http.ResponseWriter,
	r *http.Request,
	node NodeConfig,
	overridePath string,
	overrideRawQuery *string,
	overrideBody []byte,
	inspectResp func(statusCode int, body []byte),
) {
	reqPath := r.URL.Path
	if overridePath != "" {
		reqPath = overridePath
	}
	rawQuery := r.URL.RawQuery
	if overrideRawQuery != nil {
		rawQuery = *overrideRawQuery
	}
	targetURL, err := buildNodeURL(node.BaseURL, reqPath, rawQuery)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "invalid node url")
		return
	}

	var bodyReader io.Reader
	if overrideBody != nil {
		bodyReader = bytes.NewReader(overrideBody)
	} else if r.Body != nil {
		bodyReader = r.Body
	}

	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, bodyReader)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to construct upstream request")
		return
	}
	h.applyNodeHeaders(outReq.Header, r.Header, node)
	if overrideBody != nil {
		outReq.ContentLength = int64(len(overrideBody))
		outReq.Header.Set("Content-Length", strconv.Itoa(len(overrideBody)))
	} else if r.ContentLength >= 0 {
		outReq.ContentLength = r.ContentLength
	}

	// Pick streaming client for streaming endpoints or file downloads; RPC client otherwise.
	client := h.rpcClient
	if strings.Contains(reqPath, "Stream") || strings.Contains(reqPath, "stream") ||
		strings.HasPrefix(reqPath, "/api/v1/files/") || strings.HasPrefix(reqPath, "/api/v1/attachments") {
		client = h.streamClient
	}

	resp, err := client.Do(outReq)
	if err != nil {
		h.markNodeError(node.ID, err)
		writeJSONError(w, http.StatusBadGateway, fmt.Sprintf("node %s unreachable: %v", node.Name, err))
		return
	}
	defer resp.Body.Close()

	if inspectResp != nil {
		respBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		if readErr != nil {
			writeJSONError(w, http.StatusBadGateway, "failed to read node response")
			return
		}
		inspectResp(resp.StatusCode, respBytes)
		copyResponseHeaders(w.Header(), resp.Header, true)
		w.Header().Set("Content-Length", strconv.Itoa(len(respBytes)))
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(respBytes)
		return
	}

	copyResponseHeaders(w.Header(), resp.Header, false)
	w.WriteHeader(resp.StatusCode)

	flusher, canFlush := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := w.Write(buf[:n]); wErr != nil {
				return
			}
			if canFlush {
				flusher.Flush()
			}
		}
		if readErr != nil {
			return
		}
	}
}

func copyResponseHeaders(dst, src http.Header, stripContentLength bool) {
	for k, vals := range src {
		if isHopByHopHeader(k) {
			continue
		}
		if stripContentLength && strings.EqualFold(k, "Content-Length") {
			continue
		}
		dst.Del(k)
		for _, v := range vals {
			dst.Add(k, v)
		}
	}
}

// probeNode fetches /gateway/status?summary=1 from a single remote node within the short probe timeout.
func (h *Hub) probeNode(ctx context.Context, node NodeConfig) NodeStatus {
	start := time.Now()
	st := NodeStatus{
		ID:      node.ID,
		Name:    node.Name,
		BaseURL: node.BaseURL,
		OS:      node.OS,
		Version: node.Version,
		Static:  node.Static,
		Enabled: node.Enabled,
	}
	if !node.Enabled {
		st.LastError = "disabled"
		return st
	}

	pctx, cancel := context.WithTimeout(ctx, h.probeTimeout)
	defer cancel()

	code, _, body, err := h.doNodeRequest(pctx, h.probeClient, node, http.MethodGet, "/gateway/status", "summary=1", nil, "")
	if err != nil {
		st.LastError = err.Error()
		return st
	}
	if code != http.StatusOK {
		st.LastError = fmt.Sprintf("HTTP %d", code)
		return st
	}

	var gwStatus proxy.GatewayStatus
	if err := json.Unmarshal(body, &gwStatus); err != nil {
		st.LastError = "invalid status json"
		return st
	}
	st.Online = true
	st.LSHealthy = strings.EqualFold(gwStatus.Status, "connected")
	st.LatencyMs = time.Since(start).Milliseconds()
	st.LastSeen = time.Now()
	st.Summary = gwStatus.Summary
	if gwStatus.OS != "" {
		st.OS = gwStatus.OS
	}
	if st.Name == "" && gwStatus.GatewayName != "" {
		st.Name = gwStatus.GatewayName
	}
	return st
}
