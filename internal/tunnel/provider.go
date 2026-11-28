package tunnel

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/relay"
)

const (
	// ModeCloudflare uses a Cloudflare Tunnel managed by cloudflared.
	ModeCloudflare = "cloudflare"
	// ModeRelayV1 uses the embedded zero-knowledge E2EE reverse-relay pool (mgy-relay).
	ModeRelayV1 = "relay_v1"
)

// Provider abstracts a public tunnel transport (Cloudflare Tunnel or embedded E2EE relay pool).
type Provider interface {
	Start(ctx context.Context) error
	Stop()
	PublicURL() string
	Subdomain() string
	Mode() string
}

// RegisterOptions carries forward-compatible client metadata sent to the tunnel dispatcher.
type RegisterOptions struct {
	ClientVersion string
	Capabilities  []string
	SkipCache     bool
}

// ProviderOptions carries optional runtime callbacks and listeners for constructing tunnel providers.
type ProviderOptions struct {
	RelayListener     *relay.StreamListener
	LocalAddr         string
	RefreshFunc       func(ctx context.Context) ([]relay.NodeConfig, error)
	OnEndpointChanged func(node relay.NodeConfig)
	OnConnected       func(node relay.NodeConfig)
	OnAllFailed       func()
}

var (
	clientVersionMu sync.RWMutex
	clientVersion   string

	// ensureCloudflaredBinaryFn is overridable in tests to verify lazy binary provisioning.
	ensureCloudflaredBinaryFn = EnsureCloudflaredBinary
)

// SetClientVersion records the running gateway version so tunnel registration can advertise it to the dispatcher.
func SetClientVersion(v string) {
	clientVersionMu.Lock()
	defer clientVersionMu.Unlock()
	clientVersion = strings.TrimSpace(v)
}

// GetClientVersion returns the configured gateway version string, if any.
func GetClientVersion() string {
	clientVersionMu.RLock()
	defer clientVersionMu.RUnlock()
	return clientVersion
}

// SupportedCapabilities returns the tunnel transport modes supported by this gateway build.
func SupportedCapabilities() []string {
	return []string{ModeRelayV1, ModeCloudflare}
}

// EffectiveMode returns the normalized transport mode for this tunnel result, defaulting to ModeCloudflare.
func (r *CFTunnelResult) EffectiveMode() string {
	if r == nil {
		return ModeCloudflare
	}
	m := strings.ToLower(strings.TrimSpace(r.Mode))
	if m == "" {
		return ModeCloudflare
	}
	return m
}

// NewProvider constructs the matching Provider for a dispatched tunnel result with default options.
// Importantly, cloudflared binary provisioning only occurs when mode == ModeCloudflare,
// avoiding unnecessary 65MB binary downloads once embedded relay_v1 activates.
func NewProvider(ctx context.Context, res *CFTunnelResult, cfCfg *config.CloudflareConfig) (Provider, error) {
	return NewProviderWithOptions(ctx, res, cfCfg, ProviderOptions{})
}

// NewProviderWithOptions constructs the matching Provider with explicit listener/failover callbacks.
func NewProviderWithOptions(ctx context.Context, res *CFTunnelResult, cfCfg *config.CloudflareConfig, opts ProviderOptions) (Provider, error) {
	if res == nil {
		return nil, fmt.Errorf("missing tunnel dispatch result")
	}
	switch mode := res.EffectiveMode(); mode {
	case ModeCloudflare:
		binPath, err := ensureCloudflaredBinaryFn(ctx)
		if err != nil {
			return nil, fmt.Errorf("prepare cloudflared: %w", err)
		}
		return &managedCloudflareProvider{
			tunnel:  NewCloudflareTunnel(res, cfCfg),
			binPath: binPath,
		}, nil
	case ModeRelayV1:
		nodes := append([]relay.NodeConfig(nil), res.RelayNodes...)
		if len(nodes) == 0 && strings.TrimSpace(res.RelayEndpoint) != "" && strings.TrimSpace(res.Token) != "" && strings.TrimSpace(res.URL) != "" {
			healthURL := strings.Replace(res.RelayEndpoint, "wss://", "https://", 1)
			healthURL = strings.Replace(healthURL, "ws://", "http://", 1)
			healthURL = strings.Replace(healthURL, "/_mgy_tunnel", "/healthz", 1)
			nodes = []relay.NodeConfig{{
				ID:        "default",
				Region:    res.Region,
				Priority:  10,
				Endpoint:  res.RelayEndpoint,
				HealthURL: healthURL,
				Subdomain: res.Subdomain,
				URL:       res.URL,
				Token:     res.Token,
			}}
		}
		return relay.NewPoolProvider(relay.PoolConfig{
			Nodes:             nodes,
			Listener:          opts.RelayListener,
			LocalAddr:         opts.LocalAddr,
			RefreshFunc:       opts.RefreshFunc,
			OnEndpointChanged: opts.OnEndpointChanged,
			OnConnected:       opts.OnConnected,
			OnAllFailed:       opts.OnAllFailed,
		})
	default:
		return nil, fmt.Errorf("unsupported tunnel mode %q", mode)
	}
}

type managedCloudflareProvider struct {
	tunnel  *CloudflareTunnel
	binPath string
}

func (p *managedCloudflareProvider) Start(ctx context.Context) error {
	return p.tunnel.Start(ctx, p.binPath)
}

func (p *managedCloudflareProvider) Stop() {
	p.tunnel.Stop()
}

func (p *managedCloudflareProvider) PublicURL() string {
	return p.tunnel.PublicURL()
}

func (p *managedCloudflareProvider) Subdomain() string {
	return p.tunnel.Subdomain()
}

func (p *managedCloudflareProvider) Mode() string {
	return p.tunnel.Mode()
}
