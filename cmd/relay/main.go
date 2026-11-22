package main

import (
	"context"
	"crypto/tls"
	"flag"
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
	listenAddr := flag.String("listen", envOrDefault("MGY_RELAY_LISTEN", ":443"), "HTTPS listen address")
	tlsCert := flag.String("tls-cert", envOrDefault("MGY_RELAY_TLS_CERT", "/etc/mgy-relay/origin.crt"), "TLS certificate path (empty for plain HTTP)")
	tlsKey := flag.String("tls-key", envOrDefault("MGY_RELAY_TLS_KEY", "/etc/mgy-relay/origin.key"), "TLS private key path")
	controlHosts := flag.String("control-hosts", envOrDefault("MGY_RELAY_CONTROL_HOSTS", "relay.jiuge.space"), "Comma-separated control hostnames")
	allowedSuffixes := flag.String("allowed-suffixes", envOrDefault("MGY_RELAY_ALLOWED_SUFFIXES", ".jiuge.space"), "Comma-separated allowed tunnel domain suffixes")
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
			logger.Info("mgy-relay starting (plain HTTP)", "listen", *listenAddr, "control_hosts", *controlHosts)
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

func envOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
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
