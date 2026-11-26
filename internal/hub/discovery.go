package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"runtime"
	"strings"
	"time"
)

const (
	pktTypeHubProbe     = "mgy_hub_probe"
	pktTypeHubAnnounce  = "mgy_hub_announce"
	pktTypeNodeAnnounce = "mgy_node_announce"
)

// DiscoveryPacket is the compact JSON UDP datagram exchanged on the LAN discovery port.
// Secrets are NEVER transmitted in plaintext; only a non-reversible ClusterHash is included.
type DiscoveryPacket struct {
	Type        string `json:"type"`
	NodeID      string `json:"node_id,omitempty"`
	HubID       string `json:"hub_id,omitempty"`
	Name        string `json:"name,omitempty"`
	OS          string `json:"os,omitempty"`
	Port        int    `json:"port,omitempty"`
	Version     string `json:"version,omitempty"`
	LSHealthy   bool   `json:"ls_healthy,omitempty"`
	ClusterHash string `json:"cluster_hash,omitempty"`
	HubURL      string `json:"hub_url,omitempty"`
	TS          int64  `json:"ts,omitempty"`
}

// ParseDiscoveryPacket decodes and validates a UDP LAN discovery packet.
func ParseDiscoveryPacket(raw []byte) (DiscoveryPacket, bool) {
	if len(raw) == 0 || len(raw) > 2048 {
		return DiscoveryPacket{}, false
	}
	var pkt DiscoveryPacket
	if err := json.Unmarshal(raw, &pkt); err != nil {
		return DiscoveryPacket{}, false
	}
	switch pkt.Type {
	case pktTypeHubProbe, pktTypeHubAnnounce, pktTypeNodeAnnounce:
		return pkt, true
	default:
		return DiscoveryPacket{}, false
	}
}

// lanBroadcastAddrs enumerates IPv4 subnet directed broadcast addresses plus 255.255.255.255.
func lanBroadcastAddrs(port int) []*net.UDPAddr {
	seen := make(map[string]struct{})
	var out []*net.UDPAddr
	add := func(ip net.IP) {
		v4 := ip.To4()
		if v4 == nil {
			return
		}
		key := v4.String()
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, &net.UDPAddr{IP: v4, Port: port})
	}

	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagBroadcast == 0 {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				ipNet, ok := addr.(*net.IPNet)
				if !ok {
					continue
				}
				ip4 := ipNet.IP.To4()
				if ip4 == nil || len(ipNet.Mask) != 4 {
					continue
				}
				bcast := make(net.IP, 4)
				for i := 0; i < 4; i++ {
					bcast[i] = ip4[i] | ^ipNet.Mask[i]
				}
				add(bcast)
			}
		}
	}
	add(net.IPv4bcast)
	return out
}

// startHubDiscovery runs the Hub's UDP listener and periodic LAN discovery probes.
func (h *Hub) startHubDiscovery(ctx context.Context) {
	if h.opts.DisableDiscovery {
		return
	}
	port := h.opts.DiscoveryPort
	if port <= 0 {
		port = DefaultDiscoveryPort
	}

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: port})
	if err != nil {
		slog.Warn("[Hub] LAN UDP discovery listener unavailable (static nodes & POST /api/v1/hub/register still work)",
			"port", port, "err", err)
		return
	}

	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	// Periodic probe + hub announcement broadcaster
	go func() {
		sendProbe := func() {
			pkt := DiscoveryPacket{
				Type:        pktTypeHubProbe,
				HubID:       h.hubID,
				Name:        h.hubName,
				OS:          runtime.GOOS,
				ClusterHash: ClusterFingerprint(h.store.Secret()),
				HubURL:      h.opts.AdvertiseURL,
				TS:          time.Now().Unix(),
			}
			data, err := json.Marshal(pkt)
			if err != nil {
				return
			}
			for _, target := range lanBroadcastAddrs(port) {
				_, _ = conn.WriteToUDP(data, target)
			}
		}

		sendProbe()
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sendProbe()
			}
		}
	}()

	// Receive loop
	go func() {
		buf := make([]byte, 2048)
		for {
			n, remote, err := conn.ReadFromUDP(buf)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				continue
			}
			pkt, ok := ParseDiscoveryPacket(buf[:n])
			if !ok {
				continue
			}
			h.handleDiscoveryPacket(pkt, remote)
		}
	}()
}

func (h *Hub) handleDiscoveryPacket(pkt DiscoveryPacket, remote *net.UDPAddr) {
	if pkt.Type != pktTypeNodeAnnounce || remote == nil || remote.IP == nil {
		return
	}
	if pkt.Port <= 0 || pkt.Port > 65535 {
		return
	}
	nodeID := SanitizeNodeID(pkt.NodeID)
	if nodeID == "" || nodeID == h.hubID || strings.EqualFold(nodeID, LocalNodeID) {
		return
	}
	ip := remote.IP.To4()
	if ip == nil {
		return
	}
	baseURL := fmt.Sprintf("http://%s:%d", ip.String(), pkt.Port)
	name := strings.TrimSpace(pkt.Name)
	if name == "" {
		name = nodeID
	}

	wantHash := ClusterFingerprint(h.store.Secret())
	secretMatch := wantHash != "" && pkt.ClusterHash != "" && ConstantTimeEquals(pkt.ClusterHash, wantHash)

	h.discMu.Lock()
	h.discovered[nodeID] = DiscoveredNode{
		NodeID:         nodeID,
		Name:           name,
		OS:             pkt.OS,
		BaseURL:        baseURL,
		Port:           pkt.Port,
		Version:        pkt.Version,
		LSHealthy:      pkt.LSHealthy,
		ClusterHash:    pkt.ClusterHash,
		HasSecretMatch: secretMatch,
		LastSeen:       time.Now(),
	}
	h.discMu.Unlock()

	if secretMatch {
		if existing, exists := h.store.GetNode(nodeID); !exists || existing.BaseURL != baseURL || existing.Name != name {
			token := DeriveNodeToken(h.store.Secret(), nodeID)
			cfg, err := h.store.UpsertNode(NodeConfig{
				ID:      nodeID,
				Name:    name,
				BaseURL: baseURL,
				Token:   token,
				OS:      pkt.OS,
				Version: pkt.Version,
				Static:  false,
				Enabled: true,
			})
			if err == nil {
				slog.Info("[Hub] Auto-discovered & joined LAN node via shared cluster secret",
					"node_id", cfg.ID, "name", cfg.Name, "url", cfg.BaseURL)
				go h.probeNodeAndRefresh(cfg)
			}
		}
	}
}

// NodeAnnouncerConfig configures UDP discovery announcement for a LAN mgy node.
type NodeAnnouncerConfig struct {
	NodeID        string
	NodeName      string
	HTTPPort      int
	Version       string
	DiscoveryPort int
	ClusterSecret string
	IsLSHealthy   func() bool
	OnHubFound    func(hubURL, hubName string)
}

// StartNodeDiscoveryResponder advertises this mgy node on the LAN via UDP and replies to Hub probes.
func StartNodeDiscoveryResponder(ctx context.Context, cfg NodeAnnouncerConfig) {
	port := cfg.DiscoveryPort
	if port <= 0 {
		port = DefaultDiscoveryPort
	}
	if cfg.HTTPPort <= 0 {
		return
	}

	// Bind on the discovery port if available; if another local instance (e.g. a Hub on the same host)
	// already owns the port, fall back to an ephemeral UDP socket so periodic announcements still work.
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: port})
	listeningOnDiscoveryPort := err == nil
	if err != nil {
		conn, err = net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
		if err != nil {
			slog.Warn("[HubNode] UDP discovery unavailable", "err", err)
			return
		}
	}

	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	buildPacket := func() []byte {
		healthy := false
		if cfg.IsLSHealthy != nil {
			healthy = cfg.IsLSHealthy()
		}
		pkt := DiscoveryPacket{
			Type:        pktTypeNodeAnnounce,
			NodeID:      cfg.NodeID,
			Name:        cfg.NodeName,
			OS:          runtime.GOOS,
			Port:        cfg.HTTPPort,
			Version:     cfg.Version,
			LSHealthy:   healthy,
			ClusterHash: ClusterFingerprint(cfg.ClusterSecret),
			TS:          time.Now().Unix(),
		}
		data, _ := json.Marshal(pkt)
		return data
	}

	broadcast := func() {
		data := buildPacket()
		if len(data) == 0 {
			return
		}
		for _, target := range lanBroadcastAddrs(port) {
			_, _ = conn.WriteToUDP(data, target)
		}
		if !listeningOnDiscoveryPort {
			// Same-host Hub fallback
			_, _ = conn.WriteToUDP(data, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
		}
	}

	// Periodic announcer
	go func() {
		broadcast()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				broadcast()
			}
		}
	}()

	// Probe listener
	go func() {
		buf := make([]byte, 2048)
		wantHash := ClusterFingerprint(cfg.ClusterSecret)
		for {
			n, remote, err := conn.ReadFromUDP(buf)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				continue
			}
			pkt, ok := ParseDiscoveryPacket(buf[:n])
			if !ok {
				continue
			}
			if pkt.Type == pktTypeHubProbe || pkt.Type == pktTypeHubAnnounce {
				if data := buildPacket(); len(data) > 0 && remote != nil {
					_, _ = conn.WriteToUDP(data, remote)
				}
				if cfg.OnHubFound != nil && pkt.HubURL != "" {
					if wantHash == "" || (pkt.ClusterHash != "" && ConstantTimeEquals(pkt.ClusterHash, wantHash)) {
						cfg.OnHubFound(NormalizeBaseURL(pkt.HubURL), pkt.Name)
					}
				}
			}
		}
	}()
}
