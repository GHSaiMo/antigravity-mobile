package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"antigravity-mobile/internal/relay"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "token", "sign-token":
			if err := runTokenCmd(os.Args[2:], os.Stdout, os.Stderr); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		case "help", "-h", "--help":
			printUsage(os.Stdout)
			return
		}
	}

	listenAddr := flag.String("listen", envOrDefault("MGY_RELAY_LISTEN", ":443"), "HTTPS listen address")
	tlsCert := flag.String("tls-cert", envOrDefault("MGY_RELAY_TLS_CERT", "/etc/mgy-relay/origin.crt"), "TLS certificate path (empty for plain HTTP)")
	tlsKey := flag.String("tls-key", envOrDefault("MGY_RELAY_TLS_KEY", "/etc/mgy-relay/origin.key"), "TLS private key path")
	controlHosts := flag.String("control-hosts", envOrDefault("MGY_RELAY_CONTROL_HOSTS", "relay.jiuge.space"), "Comma-separated control hostnames")
	allowedSuffixes := flag.String("allowed-suffixes", envOrDefaultAllowEmpty("MGY_RELAY_ALLOWED_SUFFIXES", ".jiuge.space"), "Comma-separated allowed tunnel domain suffixes (empty allows any non-reserved domain)")
	reservedHosts := flag.String("reserved-hosts", strings.TrimSpace(os.Getenv("MGY_RELAY_RESERVED_HOSTS")), "Comma-separated additional reserved hostnames forbidden as user tunnels")
	maxTunnels := flag.Int("max-tunnels", envIntOrDefault("MGY_RELAY_MAX_TUNNELS", 10000), "Maximum concurrent gateway tunnels")
	maxStreams := flag.Int("max-streams", envIntOrDefault("MGY_RELAY_MAX_STREAMS_PER_TUNNEL", 64), "Maximum concurrent streams per tunnel")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	secret := strings.TrimSpace(os.Getenv("MGY_RELAY_SIGNING_SECRET"))
	if secret == "" {
		logger.Error("MGY_RELAY_SIGNING_SECRET is required")
		os.Exit(1)
	}

	relaySrv, err := relay.NewServer(relay.ServerConfig{
		SigningSecret:       secret,
		ControlHosts:        splitCSV(*controlHosts),
		AllowedSuffixes:     splitCSV(*allowedSuffixes),
		ReservedHosts:       splitCSV(*reservedHosts),
		MaxTunnels:          *maxTunnels,
		MaxStreamsPerTunnel: *maxStreams,
		Logger:              logger,
	})
	if err != nil {
		logger.Error("failed to initialize relay server", "err", err)
		os.Exit(1)
	}
	defer relaySrv.Close()

	httpSrv := &http.Server{
		Addr:              *listenAddr,
		Handler:           relaySrv,
		ReadHeaderTimeout: 10 * time.Second,
		MaxHeaderBytes:    64 << 10,
		IdleTimeout:       120 * time.Second,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		var srvErr error
		if strings.TrimSpace(*tlsCert) != "" && strings.TrimSpace(*tlsKey) != "" {
			logger.Info("mgy-relay starting (TLS)", "listen", *listenAddr, "control_hosts", *controlHosts, "allowed_suffixes", *allowedSuffixes)
			srvErr = httpSrv.ListenAndServeTLS(*tlsCert, *tlsKey)
		} else {
			logger.Info("mgy-relay starting (plain HTTP)", "listen", *listenAddr, "control_hosts", *controlHosts, "allowed_suffixes", *allowedSuffixes)
			srvErr = httpSrv.ListenAndServe()
		}
		if srvErr != nil && srvErr != http.ErrServerClosed {
			logger.Error("relay server failed", "err", srvErr)
			os.Exit(1)
		}
	}()

	<-stopCh
	logger.Info("mgy-relay shutting down...")
	relaySrv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
	logger.Info("mgy-relay stopped cleanly")
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `mgy-relay - Multigravity Zero-Knowledge E2EE Reverse Relay

Usage:
  mgy-relay [server flags]
  mgy-relay token --domain <host> [--secret <secret>] [--endpoint <wss-url>] [--json]

Subcommands:
  token (sign-token)  Issue an offline signed mgyr1 token for BYO-Relay self-hosting (no Worker needed)

Server Flags:
  -listen            HTTPS/HTTP listen address (default ":443", env: MGY_RELAY_LISTEN)
  -tls-cert          TLS certificate path; empty for plain HTTP behind reverse proxy (env: MGY_RELAY_TLS_CERT)
  -tls-key           TLS private key path (env: MGY_RELAY_TLS_KEY)
  -control-hosts     Comma-separated control hostnames (env: MGY_RELAY_CONTROL_HOSTS)
  -allowed-suffixes  Comma-separated allowed tunnel domain suffixes (env: MGY_RELAY_ALLOWED_SUFFIXES)
  -reserved-hosts    Comma-separated additional reserved hostnames (env: MGY_RELAY_RESERVED_HOSTS)
  -max-tunnels       Maximum concurrent gateway tunnels (default 10000)
  -max-streams       Maximum concurrent streams per tunnel (default 64)`)
}

func runTokenCmd(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("token", flag.ContinueOnError)
	fs.SetOutput(stderr)

	domain := fs.String("domain", "", "Public tunnel hostname routed to this gateway (e.g. dev.example.com)")
	secret := fs.String("secret", strings.TrimSpace(os.Getenv("MGY_RELAY_SIGNING_SECRET")), "Relay HMAC signing secret (defaults to MGY_RELAY_SIGNING_SECRET)")
	shortID := fs.String("short-id", "", "Optional 8-char hex short ID (auto-generated if empty)")
	ownerHash := fs.String("owner-hash", "", "Optional 16..64-char hex owner hash (auto-generated if empty)")
	region := fs.String("region", "self", "Optional region label (e.g. self, home, office)")
	endpoint := fs.String("endpoint", "", "Optional control endpoint URL for env snippet (e.g. wss://relay.example.com/_mgy_tunnel)")
	jsonOut := fs.Bool("json", false, "Emit machine-readable JSON only")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cleanDomain := strings.ToLower(strings.TrimSpace(*domain))
	cleanDomain = strings.TrimPrefix(cleanDomain, "https://")
	cleanDomain = strings.TrimPrefix(cleanDomain, "http://")
	cleanDomain = strings.TrimRight(cleanDomain, "/")
	if cleanDomain == "" {
		return fmt.Errorf("--domain is required (e.g. --domain dev.example.com)")
	}
	cleanSecret := strings.TrimSpace(*secret)
	if cleanSecret == "" {
		return fmt.Errorf("--secret or MGY_RELAY_SIGNING_SECRET is required")
	}

	sid := strings.ToLower(strings.TrimSpace(*shortID))
	if sid == "" {
		var err error
		sid, err = randomHexBytes(4)
		if err != nil {
			return fmt.Errorf("generate short-id: %w", err)
		}
	}
	oh := strings.ToLower(strings.TrimSpace(*ownerHash))
	if oh == "" {
		var err error
		oh, err = randomHexBytes(16)
		if err != nil {
			return fmt.Errorf("generate owner-hash: %w", err)
		}
	}

	claims := relay.TokenClaims{
		Version:   1,
		ShortID:   sid,
		OwnerHash: oh,
		Domain:    cleanDomain,
		Region:    strings.ToLower(strings.TrimSpace(*region)),
		IssuedAt:  time.Now().Unix(),
	}
	tok, err := relay.SignToken(cleanSecret, claims)
	if err != nil {
		return err
	}
	// Self-verify so invalid --domain / --short-id / --owner-hash fail immediately.
	if _, err := relay.VerifyToken(cleanSecret, tok, time.Now(), 0); err != nil {
		return fmt.Errorf("invalid token parameters: %w", err)
	}

	publicURL := "https://" + cleanDomain
	ep := strings.TrimSpace(*endpoint)
	if ep == "" {
		if ctrlHosts := splitCSV(os.Getenv("MGY_RELAY_CONTROL_HOSTS")); len(ctrlHosts) > 0 {
			ep = "wss://" + ctrlHosts[0] + relay.TunnelUpgradePath
		} else {
			ep = "wss://<your-relay-control-host>" + relay.TunnelUpgradePath
		}
	}

	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"token":      tok,
			"domain":     cleanDomain,
			"public_url": publicURL,
			"short_id":   sid,
			"owner_hash": oh,
			"region":     claims.Region,
			"env": map[string]string{
				"MGY_CUSTOM_RELAY_ENDPOINT":   ep,
				"MGY_CUSTOM_RELAY_TOKEN":      tok,
				"MGY_CUSTOM_RELAY_PUBLIC_URL": publicURL,
			},
		})
	}

	fmt.Fprintf(stdout, "%s\n", tok)
	fmt.Fprintf(stderr, "\n# Add to your gateway ~/.multigravity/.env (BYO-Relay self-hosted mode):\n")
	fmt.Fprintf(stderr, "MGY_CUSTOM_RELAY_ENDPOINT=%s\n", ep)
	fmt.Fprintf(stderr, "MGY_CUSTOM_RELAY_TOKEN=%s\n", tok)
	fmt.Fprintf(stderr, "MGY_CUSTOM_RELAY_PUBLIC_URL=%s\n", publicURL)
	return nil
}

func randomHexBytes(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func envOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envOrDefaultAllowEmpty(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return strings.TrimSpace(v)
	}
	return fallback
}

func envIntOrDefault(key string, fallback int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

func splitCSV(raw string) []string {
	var out []string
	for _, item := range strings.Split(raw, ",") {
		if s := strings.TrimSpace(item); s != "" {
			out = append(out, s)
		}
	}
	return out
}
