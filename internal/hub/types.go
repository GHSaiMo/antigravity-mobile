package hub

import (
	"time"

	"antigravity-mobile/internal/inspector"
	"antigravity-mobile/internal/notifier"
	"antigravity-mobile/internal/proxy"
)

const (
	// LocalNodeID identifies the Hub's own local Antigravity instance when not running headless.
	LocalNodeID = "local"

	// HeaderHubForwarded marks internal Hub-to-Node requests to prevent infinite forwarding loops.
	HeaderHubForwarded = "X-Mgy-Hub-Forwarded"

	// HeaderHubToken carries the shared node authentication token between Hub and LAN nodes.
	HeaderHubToken = "X-Mgy-Hub-Token"

	// HeaderHubNodeID optionally carries the originating node ID on internal callbacks.
	HeaderHubNodeID = "X-Mgy-Hub-Node-Id"

	// HeaderHubURL optionally carries the authenticated Hub's current LAN advertise URL
	// so linked nodes can keep push/Live Activity event forwarding accurate across DHCP changes.
	HeaderHubURL = "X-Mgy-Hub-URL"

	// DefaultDiscoveryPort is the default IPv4 UDP port used for LAN node discovery.
	DefaultDiscoveryPort = 58901

	// DefaultProbeTimeout is the short LAN timeout so offline/sleeping nodes never stall the Hub.
	DefaultProbeTimeout = 800 * time.Millisecond

	// DefaultListTimeout is the bounded timeout when fanning out list/project/search calls across LAN nodes.
	DefaultListTimeout = 1500 * time.Millisecond

	// DefaultProbeInterval is how frequently the Hub probes registered nodes for health & summary counters.
	DefaultProbeInterval = 5 * time.Second
)

// NodeConfig represents a persisted remote LAN mgy node registered with the Hub.
type NodeConfig struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	BaseURL string    `json:"base_url"`
	Token   string    `json:"token,omitempty"`
	OS      string    `json:"os,omitempty"`
	Version string    `json:"version,omitempty"`
	Static  bool      `json:"static,omitempty"`
	Enabled bool      `json:"enabled"`
	AddedAt time.Time `json:"added_at"`
}

// NodeStatus is the live runtime snapshot of a registered node returned by Hub status APIs.
type NodeStatus struct {
	ID           string                `json:"id"`
	Name         string                `json:"name"`
	BaseURL      string                `json:"base_url,omitempty"`
	OS           string                `json:"os,omitempty"`
	Version      string                `json:"version,omitempty"`
	Local        bool                  `json:"local,omitempty"`
	Static       bool                  `json:"static,omitempty"`
	Enabled      bool                  `json:"enabled"`
	Online       bool                  `json:"online"`
	LSHealthy    bool                  `json:"ls_healthy"`
	LatencyMs    int64                 `json:"latency_ms,omitempty"`
	LastSeen     time.Time             `json:"last_seen,omitempty"`
	LastError    string                `json:"last_error,omitempty"`
	CascadeCount int                   `json:"cascade_count"`
	Summary      *proxy.GatewaySummary `json:"summary,omitempty"`
}

// DiscoveredNode represents a LAN mgy gateway heard over UDP discovery that has not yet been paired/approved.
type DiscoveredNode struct {
	NodeID           string    `json:"node_id"`
	Name             string    `json:"name"`
	OS               string    `json:"os,omitempty"`
	Mode             string    `json:"mode,omitempty"` // "standalone" | "node"
	BaseURL          string    `json:"base_url"`
	Port             int       `json:"port"`
	Version          string    `json:"version,omitempty"`
	LSHealthy        bool      `json:"ls_healthy"`
	ClusterHash      string    `json:"cluster_hash,omitempty"`
	HasSecretMatch   bool      `json:"has_secret_match"`
	RequiresPairCode bool      `json:"requires_pair_code"`
	LastSeen         time.Time `json:"last_seen"`
}

// HubStatusResponse describes the aggregated runtime state of the Hub cluster.
type HubStatusResponse struct {
	Mode            string           `json:"mode"`
	Headless        bool             `json:"headless"`
	HubID           string           `json:"hub_id"`
	HubName         string           `json:"hub_name"`
	ActiveFilter    string           `json:"active_filter"`
	JoinCode        string           `json:"join_code,omitempty"`
	SecretSet       bool             `json:"secret_set"`
	LocalOnline     bool             `json:"local_online"`
	LocalLSHealthy  bool             `json:"local_ls_healthy"`
	OnlineNodeCount int              `json:"online_node_count"`
	TotalNodeCount  int              `json:"total_node_count"`
	Nodes           []NodeStatus     `json:"nodes"`
	Discovered      []DiscoveredNode `json:"discovered,omitempty"`
	Timestamp       time.Time        `json:"timestamp"`
}

// RegisterNodeRequest is sent by a LAN mgy node to POST /api/v1/hub/register.
type RegisterNodeRequest struct {
	NodeID    string `json:"node_id"`
	Name      string `json:"name"`
	BaseURL   string `json:"base_url,omitempty"`
	Port      int    `json:"port,omitempty"`
	OS        string `json:"os,omitempty"`
	Version   string `json:"version,omitempty"`
	Secret    string `json:"secret,omitempty"`
	JoinCode  string `json:"join_code,omitempty"`
	NodeToken string `json:"node_token,omitempty"`
}

// RegisterNodeResponse is returned by POST /api/v1/hub/register on success.
type RegisterNodeResponse struct {
	Status    string `json:"status"`
	HubID     string `json:"hub_id"`
	HubName   string `json:"hub_name"`
	NodeID    string `json:"node_id"`
	NodeName  string `json:"node_name"`
	NodeToken string `json:"node_token"`
}

// NodeEventPayload carries real-time notifications, Live Activity updates, and list change signals
// forwarded from a remote LAN node to the Hub via POST /api/v1/hub/internal/event.
type NodeEventPayload struct {
	NodeID    string `json:"node_id"`
	NodeName  string `json:"node_name,omitempty"`
	Type      string `json:"type"` // "push" | "live_activity" | "live_activity_end" | "list_changed"
	CascadeID string `json:"cascade_id,omitempty"`

	// Push alert fields (when Type == "push")
	Title     string               `json:"title,omitempty"`
	Body      string               `json:"body,omitempty"`
	URL       string               `json:"url,omitempty"`
	Level     string               `json:"level,omitempty"`
	Sound     string               `json:"sound,omitempty"`
	Category  string               `json:"category,omitempty"`
	Silent    bool                 `json:"silent,omitempty"`
	Action    string               `json:"action,omitempty"`
	Localized map[string][2]string `json:"localized,omitempty"` // lang -> [title, body]

	// Live Activity fields (when Type == "live_activity" or "live_activity_end")
	Details    *proxy.TrajectoryDetails `json:"details,omitempty"`
	EndState   string                   `json:"end_state,omitempty"`
	TotalSteps int                      `json:"total_steps,omitempty"`
}

// HubLink represents a Hub authorized on this machine via secondary confirmation code pairing.
// Persisted on the target mgy host at ~/.multigravity/hub_links.json (0600).
type HubLink struct {
	HubID      string    `json:"hub_id"`
	HubName    string    `json:"hub_name"`
	HubURL     string    `json:"hub_url,omitempty"`
	NodeToken  string    `json:"node_token"`
	PairedAt   time.Time `json:"paired_at"`
	LastSeenAt time.Time `json:"last_seen_at,omitempty"`
}

// HubLinkView is the token-redacted public view of HubLink returned by GET /api/v1/hub/links.
type HubLinkView struct {
	HubID      string    `json:"hub_id"`
	HubName    string    `json:"hub_name"`
	HubURL     string    `json:"hub_url,omitempty"`
	PairedAt   time.Time `json:"paired_at"`
	LastSeenAt time.Time `json:"last_seen_at,omitempty"`
}

// HubPairRequestPayload is sent by a LAN Hub to POST /api/v1/hub/pair-request on a target mgy host.
type HubPairRequestPayload struct {
	HubID   string `json:"hub_id"`
	HubName string `json:"hub_name"`
	HubURL  string `json:"hub_url,omitempty"`
}

// HubPairRequestResponse is returned by POST /api/v1/hub/pair-request (never includes the confirmation code).
type HubPairRequestResponse struct {
	Status      string `json:"status"`
	ChallengeID string `json:"challenge_id"`
	NodeID      string `json:"node_id"`
	NodeName    string `json:"node_name"`
	ExpiresIn   int    `json:"expires_in"`
}

// HubPairConfirmPayload is sent by a LAN Hub to POST /api/v1/hub/pair-confirm with the 6-char code.
type HubPairConfirmPayload struct {
	ChallengeID string `json:"challenge_id,omitempty"`
	Code        string `json:"code"`
	HubID       string `json:"hub_id"`
	HubName     string `json:"hub_name"`
	HubURL      string `json:"hub_url,omitempty"`
}

// HubPairConfirmResponse is returned by POST /api/v1/hub/pair-confirm upon successful code verification.
type HubPairConfirmResponse struct {
	Status    string `json:"status"`
	NodeID    string `json:"node_id"`
	NodeName  string `json:"node_name"`
	NodeToken string `json:"node_token"`
	OS        string `json:"os,omitempty"`
	Version   string `json:"version,omitempty"`
}

// HubPairCodeResponse is returned locally to `mgy pair -hub` by POST /api/v1/hub/pair-code.
type HubPairCodeResponse struct {
	Code          string `json:"code"`
	FormattedCode string `json:"formatted_code"`
	NodeID        string `json:"node_id"`
	NodeName      string `json:"node_name"`
	LANURL        string `json:"lan_url,omitempty"`
	ExpiresIn     int    `json:"expires_in"`
}

// HubVerifyRequest is sent by a Hub to POST /api/v1/hub/verify when verifying a registered node at a new LAN IP.
// No token is ever sent on the wire; the node proves possession of nodeToken via HMAC-SHA256.
type HubVerifyRequest struct {
	HubID string `json:"hub_id"`
	Nonce string `json:"nonce"`
}

// HubVerifyResponse carries the HMAC-SHA256 proof returned by POST /api/v1/hub/verify.
type HubVerifyResponse struct {
	NodeID   string `json:"node_id"`
	NodeName string `json:"node_name"`
	Proof    string `json:"proof"`
}

// Options configures a new Hub instance.
type Options struct {
	HubID            string
	HubName          string
	Headless         bool
	LocalPort        int
	DataDir          string
	Secret           string
	StaticNodes      []string // format: "http://ip:port", "name=http://ip:port", or "name=http://ip:port|token"
	DiscoveryPort    int
	DisableDiscovery bool
	AdvertiseURL     string // Hub LAN URL advertised via UDP (e.g. "http://192.168.50.10:58900")
	ProbeTimeout     time.Duration
	ListTimeout      time.Duration
	ProbeInterval    time.Duration
	Proxy            *proxy.Proxy
	Inspector        inspector.UpstreamDiscoverer
	Notifier         *notifier.Notifier
}
