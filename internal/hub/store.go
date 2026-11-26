package hub

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	hubNodesFilename = "hub_nodes.json"
	hubNodeFilename  = "hub_node.json"
)

var safeNodeIDRe = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// persistedHubState is saved to ~/.multigravity/hub_nodes.json (0600).
type persistedHubState struct {
	HubSecret    string       `json:"hub_secret,omitempty"`
	ActiveFilter string       `json:"active_filter,omitempty"`
	Nodes        []NodeConfig `json:"nodes"`
}

// NodeCredentials is saved on a LAN node at ~/.multigravity/hub_node.json (0600).
type NodeCredentials struct {
	NodeID    string `json:"node_id"`
	NodeName  string `json:"node_name"`
	HubURL    string `json:"hub_url,omitempty"`
	NodeToken string `json:"node_token,omitempty"`
	HubSecret string `json:"hub_secret,omitempty"`
}

// Store manages persisted Hub cluster nodes, shared secret, join code, and active node filter.
type Store struct {
	mu           sync.RWMutex
	path         string
	hubSecret    string
	joinCode     string
	activeFilter string
	nodes        map[string]NodeConfig
}

// NewStore loads or initializes the Hub cluster store under dataDir.
func NewStore(dataDir, secretOverride string) (*Store, error) {
	if strings.TrimSpace(dataDir) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home dir: %w", err)
		}
		dataDir = filepath.Join(home, ".multigravity")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create hub data dir: %w", err)
	}

	s := &Store{
		path:     filepath.Join(dataDir, hubNodesFilename),
		joinCode: generateJoinCode(6),
		nodes:    make(map[string]NodeConfig),
	}

	if raw, err := os.ReadFile(s.path); err == nil && len(raw) > 0 {
		var state persistedHubState
		if err := json.Unmarshal(raw, &state); err == nil {
			s.hubSecret = strings.TrimSpace(state.HubSecret)
			s.activeFilter = normalizeFilter(state.ActiveFilter)
			for _, n := range state.Nodes {
				if id := SanitizeNodeID(n.ID); id != "" && strings.TrimSpace(n.BaseURL) != "" {
					n.ID = id
					n.BaseURL = NormalizeBaseURL(n.BaseURL)
					if strings.TrimSpace(n.Name) == "" {
						n.Name = n.ID
					}
					s.nodes[id] = n
				}
			}
		}
	}

	changed := false
	if sec := strings.TrimSpace(secretOverride); sec != "" {
		if s.hubSecret != sec {
			s.hubSecret = sec
			changed = true
		}
	} else if s.hubSecret == "" {
		s.hubSecret = randomHex(24)
		changed = true
	}

	if changed {
		_ = s.saveLocked()
	}
	return s, nil
}

func (s *Store) saveLocked() error {
	list := make([]NodeConfig, 0, len(s.nodes))
	for _, n := range s.nodes {
		list = append(list, n)
	}
	state := persistedHubState{
		HubSecret:    s.hubSecret,
		ActiveFilter: s.activeFilter,
		Nodes:        list,
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Secret returns the Hub's shared cluster secret.
func (s *Store) Secret() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hubSecret
}

// JoinCode returns the current interactive LAN join code.
func (s *Store) JoinCode() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.joinCode
}

// RotateJoinCode replaces the interactive join code and returns the new code.
func (s *Store) RotateJoinCode() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.joinCode = generateJoinCode(6)
	return s.joinCode
}

// ActiveFilter returns "" (all) or the selected node ID.
func (s *Store) ActiveFilter() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activeFilter
}

// SetActiveFilter sets the active node filter ("" or "all" resets to all nodes).
func (s *Store) SetActiveFilter(filter string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeFilter = normalizeFilter(filter)
	_ = s.saveLocked()
	return s.activeFilter
}

// UpsertNode adds or updates a remote node in the cluster store.
func (s *Store) UpsertNode(cfg NodeConfig) (NodeConfig, error) {
	cfg.BaseURL = NormalizeBaseURL(cfg.BaseURL)
	if cfg.BaseURL == "" {
		return NodeConfig{}, fmt.Errorf("base_url is required")
	}
	if strings.TrimSpace(cfg.Name) == "" {
		if u, err := url.Parse(cfg.BaseURL); err == nil && u.Hostname() != "" {
			cfg.Name = u.Hostname()
		} else {
			cfg.Name = "node"
		}
	}
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.ID = SanitizeNodeID(cfg.ID)
	if cfg.ID == "" || strings.EqualFold(cfg.ID, LocalNodeID) {
		cfg.ID = SanitizeNodeID(cfg.Name)
		if cfg.ID == "" || strings.EqualFold(cfg.ID, LocalNodeID) {
			cfg.ID = "node-" + randomHex(4)
		}
	}
	if cfg.AddedAt.IsZero() {
		cfg.AddedAt = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if prev, ok := s.nodes[cfg.ID]; ok {
		if cfg.Token == "" {
			cfg.Token = prev.Token
		}
		if cfg.OS == "" {
			cfg.OS = prev.OS
		}
		if cfg.Version == "" {
			cfg.Version = prev.Version
		}
		if !prev.AddedAt.IsZero() {
			cfg.AddedAt = prev.AddedAt
		}
	}
	if cfg.Token == "" && s.hubSecret != "" {
		cfg.Token = DeriveNodeToken(s.hubSecret, cfg.ID)
	}
	s.nodes[cfg.ID] = cfg
	if err := s.saveLocked(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// RemoveNode deletes a node by ID or name. Returns true if removed.
func (s *Store) RemoveNode(idOrName string) bool {
	key := strings.TrimSpace(idOrName)
	if key == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.nodes[key]; ok {
		delete(s.nodes, key)
		if s.activeFilter == key {
			s.activeFilter = ""
		}
		_ = s.saveLocked()
		return true
	}
	for id, n := range s.nodes {
		if strings.EqualFold(id, key) || strings.EqualFold(n.Name, key) {
			delete(s.nodes, id)
			if s.activeFilter == id {
				s.activeFilter = ""
			}
			_ = s.saveLocked()
			return true
		}
	}
	return false
}

// GetNode returns a node by exact ID or case-insensitive ID/Name match.
func (s *Store) GetNode(idOrName string) (NodeConfig, bool) {
	key := strings.TrimSpace(idOrName)
	if key == "" {
		return NodeConfig{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if n, ok := s.nodes[key]; ok {
		return n, true
	}
	for _, n := range s.nodes {
		if strings.EqualFold(n.ID, key) || strings.EqualFold(n.Name, key) {
			return n, true
		}
	}
	return NodeConfig{}, false
}

// ListNodes returns a snapshot slice of all registered remote nodes.
func (s *Store) ListNodes() []NodeConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]NodeConfig, 0, len(s.nodes))
	for _, n := range s.nodes {
		out = append(out, n)
	}
	return out
}

// ValidateNodeToken returns the matching NodeConfig if token matches any enabled node's token.
func (s *Store) ValidateNodeToken(token string) (NodeConfig, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return NodeConfig{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, n := range s.nodes {
		if !n.Enabled || n.Token == "" {
			continue
		}
		if ConstantTimeEquals(token, n.Token) {
			return n, true
		}
	}
	return NodeConfig{}, false
}

// AuthorizeJoin verifies either the Hub's shared cluster secret or the interactive join code.
func (s *Store) AuthorizeJoin(secret, joinCode string) bool {
	s.mu.RLock()
	wantSecret := s.hubSecret
	wantCode := s.joinCode
	s.mu.RUnlock()

	if sec := strings.TrimSpace(secret); sec != "" && wantSecret != "" && ConstantTimeEquals(sec, wantSecret) {
		return true
	}
	if code := strings.ToUpper(strings.TrimSpace(joinCode)); code != "" && wantCode != "" && ConstantTimeEquals(code, strings.ToUpper(wantCode)) {
		return true
	}
	return false
}

// LoadOrCreateNodeCredentials loads or creates node-side credentials at <dataDir>/hub_node.json.
func LoadOrCreateNodeCredentials(dataDir, defaultName string) (NodeCredentials, error) {
	if strings.TrimSpace(dataDir) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return NodeCredentials{}, err
		}
		dataDir = filepath.Join(home, ".multigravity")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return NodeCredentials{}, err
	}
	path := filepath.Join(dataDir, hubNodeFilename)
	var creds NodeCredentials
	if raw, err := os.ReadFile(path); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &creds)
	}
	changed := false
	if strings.TrimSpace(creds.NodeName) == "" {
		creds.NodeName = strings.TrimSpace(defaultName)
		if creds.NodeName == "" {
			if hn, err := os.Hostname(); err == nil && hn != "" {
				creds.NodeName = strings.TrimSuffix(hn, ".local")
			} else {
				creds.NodeName = "node"
			}
		}
		changed = true
	}
	if strings.TrimSpace(creds.NodeID) == "" {
		base := SanitizeNodeID(creds.NodeName)
		if base == "" || strings.EqualFold(base, LocalNodeID) {
			base = "node"
		}
		creds.NodeID = base + "-" + randomHex(3)
		changed = true
	}
	if changed {
		_ = SaveNodeCredentials(dataDir, creds)
	}
	return creds, nil
}

// SaveNodeCredentials persists node-side Hub credentials with 0600 permissions.
func SaveNodeCredentials(dataDir string, creds NodeCredentials) error {
	if strings.TrimSpace(dataDir) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dataDir = filepath.Join(home, ".multigravity")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dataDir, hubNodeFilename)
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ClusterFingerprint computes a non-reversible 16-hex-char fingerprint of a cluster secret
// for safe UDP LAN advertisement without ever putting plaintext secrets on the wire.
func ClusterFingerprint(secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("mgy-hub-cluster-v1:" + secret))
	return hex.EncodeToString(sum[:8])
}

// DeriveNodeToken derives a deterministic per-node auth token from a shared cluster secret and node ID.
func DeriveNodeToken(secret, nodeID string) string {
	mac := hmac.New(sha256.New, []byte(strings.TrimSpace(secret)))
	_, _ = mac.Write([]byte("mgy-node-v1:" + strings.TrimSpace(nodeID)))
	return "mgy_node_" + hex.EncodeToString(mac.Sum(nil)[:24])
}

// ConstantTimeEquals compares two non-empty secrets in constant time without leaking length.
func ConstantTimeEquals(got, want string) bool {
	if got == "" || want == "" {
		return false
	}
	g := sha256.Sum256([]byte(got))
	w := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(g[:], w[:]) == 1
}

// SanitizeNodeID normalizes a node ID into a safe URL/identifier-friendly token.
func SanitizeNodeID(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	s = safeNodeIDRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-._")
	if len(s) > 48 {
		s = s[:48]
	}
	return s
}

// NormalizeBaseURL ensures an http(s) node base URL has no trailing slash.
func NormalizeBaseURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

// ParseStaticNodeSpec parses specs like:
//   - "http://192.168.50.12:58900"
//   - "M1=http://192.168.50.12:58900"
//   - "M1=http://192.168.50.12:58900|tok_abc123"
//   - "192.168.50.12:58900"
func ParseStaticNodeSpec(spec string) (NodeConfig, error) {
	raw := strings.TrimSpace(spec)
	if raw == "" {
		return NodeConfig{}, fmt.Errorf("empty node spec")
	}
	var name, token string
	if idx := strings.LastIndex(raw, "|"); idx > 0 {
		token = strings.TrimSpace(raw[idx+1:])
		raw = strings.TrimSpace(raw[:idx])
	}
	if idx := strings.Index(raw, "="); idx > 0 && !strings.Contains(raw[:idx], "://") {
		name = strings.TrimSpace(raw[:idx])
		raw = strings.TrimSpace(raw[idx+1:])
	}
	baseURL := NormalizeBaseURL(raw)
	if baseURL == "" {
		return NodeConfig{}, fmt.Errorf("invalid node url: %q", spec)
	}
	if name == "" {
		if u, err := url.Parse(baseURL); err == nil {
			name = u.Hostname()
		}
	}
	id := SanitizeNodeID(name)
	if id == "" || strings.EqualFold(id, LocalNodeID) {
		id = "node-" + randomHex(4)
	}
	return NodeConfig{
		ID:      id,
		Name:    name,
		BaseURL: baseURL,
		Token:   token,
		Static:  true,
		Enabled: true,
		AddedAt: time.Now(),
	}, nil
}

func normalizeFilter(f string) string {
	f = strings.TrimSpace(f)
	if f == "" || strings.EqualFold(f, "all") || f == "*" {
		return ""
	}
	return f
}

func randomHex(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func generateJoinCode(length int) string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	out := make([]byte, length)
	max := big.NewInt(int64(len(alphabet)))
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			out[i] = alphabet[int(time.Now().UnixNano()+int64(i))%len(alphabet)]
			continue
		}
		out[i] = alphabet[n.Int64()]
	}
	return string(out)
}
