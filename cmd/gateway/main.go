package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"antigravity-mobile/internal/auth"
	"antigravity-mobile/internal/cockpit"
	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/inspector"
	"antigravity-mobile/internal/notifier"
	"antigravity-mobile/internal/proxy"
	"antigravity-mobile/web"
)

func main() {
	// 0. Load .env configuration
	config.LoadDotEnv()

	// Default to dual-stack socket binding (default "" binds to all IPv4 and IPv6 interfaces); override with -host flag or GATEWAY_HOST env var
	defaultHost := ""
	if envHost := os.Getenv("GATEWAY_HOST"); envHost != "" {
		defaultHost = envHost
	}
	defaultPort := 58900
	if envPort := os.Getenv("GATEWAY_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil && p > 0 {
			defaultPort = p
		}
	}
	host := flag.String("host", defaultHost, "Host/IP for Mobile Gateway to listen on (default \"\" binds to all IPv4 and IPv6 interfaces)")
	port := flag.Int("port", defaultPort, "Port for Mobile Gateway to listen on")
	pollSec := flag.Int("poll", 5, "Polling interval in seconds for Antigravity instance discovery")
	ddnsHost := flag.String("ddns", os.Getenv("DDNS_HOST"), "Public DDNS domain or IPv6 address for pairing QR code")
	enableSSL := flag.Bool("ssl", os.Getenv("GATEWAY_SSL") == "1" || os.Getenv("GATEWAY_SSL") == "true", "Indicate SSL mode in pairing QR code")
	tlsCert := flag.String("tls-cert", os.Getenv("TLS_CERT_FILE"), "Path to TLS certificate file for HTTPS (optional)")
	tlsKey := flag.String("tls-key", os.Getenv("TLS_KEY_FILE"), "Path to TLS private key file for HTTPS (optional)")
	flag.Parse()

	log.Printf("==================================================")
	log.Printf("🚀 Antigravity starting on :%d", *port)
	log.Printf("==================================================")

	// 1. Initialize Inspector
	insp := inspector.NewInspector(time.Duration(*pollSec) * time.Second)
	insp.Start()
	defer insp.Stop()

	// 2. Initialize Reverse Proxy & WebSocket handler
	p := proxy.NewProxy(insp)

	// 3. Initialize Auth Store & Pairing Manager
	authStorePath := os.Getenv("AUTH_STORE_PATH")
	authStore, err := auth.NewAuthStore(authStorePath)
	if err != nil {
		log.Fatalf("❌ Failed to initialize auth store: %v", err)
	}

	netAddrs := auth.DetectNetworkAddresses()
	qrHost := *ddnsHost
	var extraHosts []string

	if qrHost == "" {
		if *host != "" && *host != "0.0.0.0" && *host != "::" && *host != "[::]" {
			qrHost = *host
		} else if netAddrs.LANIPv4 != "" {
			qrHost = netAddrs.LANIPv4
			if netAddrs.PublicIPv6 != "" {
				extraHosts = append(extraHosts, netAddrs.PublicIPv6)
			}
		} else if netAddrs.PublicIPv6 != "" {
			qrHost = netAddrs.PublicIPv6
		} else {
			qrHost = "127.0.0.1"
		}
	} else {
		if netAddrs.LANIPv4 != "" && netAddrs.LANIPv4 != qrHost {
			extraHosts = append(extraHosts, netAddrs.LANIPv4)
		}
		if netAddrs.PublicIPv6 != "" && netAddrs.PublicIPv6 != qrHost {
			extraHosts = append(extraHosts, netAddrs.PublicIPv6)
		}
	}

	pairingMgr := auth.NewPairingManager()
	authHandler := auth.NewAuthHandler(authStore, pairingMgr, qrHost, *port, *enableSSL)
	authHandler.SetEndpoints(netAddrs.LANIPv4, netAddrs.PublicIPv6, *ddnsHost)

	// Print initial pairing QR code
	if initialSession, err := pairingMgr.GenerateSession(5 * time.Minute); err == nil {
		auth.PrintPairingQRCode(qrHost, *port, initialSession.Code, *enableSSL, extraHosts...)
	}

	// 4. Initialize Push Notification & Background Watcher
	notifCfg := config.GetNotificationConfig()
	watcherCtx, cancelWatcher := context.WithCancel(context.Background())
	defer cancelWatcher()

	// Periodically cleanup expired pairing sessions
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				pairingMgr.CleanupExpired()
			case <-watcherCtx.Done():
				return
			}
		}
	}()

	if notifCfg.Enabled {
		notif := notifier.NewNotifier(notifCfg)
		p.SetNotificationSink(notif)

		watcher := notifier.NewWatcher(p, notif)
		watcher.Start(watcherCtx)

		log.Printf("🔔 Bark notifications ENABLED")
		log.Printf("   🎯 Target: %s", notifCfg.BarkEndpoint)
		log.Printf("   🎨 Icon:   %s", notifCfg.IconURL)
		log.Printf("   📁 Group:  %s", notifCfg.Group)
	} else {
		log.Printf("ℹ️  Bark notifications disabled (set BARK_URL in .env to enable)")
	}

	// 5. Web frontend handler
	webHandler := web.Handler()

	// 6. Combined Root Router (Go 1.22+ ServeMux with method-aware patterns)
	rootMux := http.NewServeMux()

	// Auth endpoints
	rootMux.HandleFunc("/api/v1/auth/pair", authHandler.HandlePair)
	rootMux.HandleFunc("/api/v1/auth/session", authHandler.HandleNewPairingSession)
	rootMux.HandleFunc("/api/v1/devices/", authHandler.HandleDevices)
	rootMux.HandleFunc("/api/v1/devices", authHandler.HandleDevices)

	// Cockpit endpoints
	rootMux.HandleFunc("GET /api/v1/cockpit/quotas", func(w http.ResponseWriter, r *http.Request) {
		liveEmail, _, _ := p.GetActiveUserStatus()
		quotas, err := cockpit.GetQuotas(liveEmail)
		if err != nil {
			log.Printf("[Cockpit] GetQuotas failed: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, quotas)
	})
	rootMux.HandleFunc("POST /api/v1/cockpit/refresh", func(w http.ResponseWriter, r *http.Request) {
		log.Println("[Cockpit] Triggering quota refresh...")
		err := cockpit.TriggerRefresh()
		if err != nil {
			log.Printf("[Cockpit] TriggerRefresh failed: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		log.Println("[Cockpit] Quota refresh triggered successfully")
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "refresh triggered"})
	})
	rootMux.HandleFunc("POST /api/v1/cockpit/switch", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			AccountID string `json:"account_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.AccountID) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "account_id is required"})
			return
		}
		targetID := strings.TrimSpace(req.AccountID)
		log.Printf("[Cockpit] Switching account to: %s", targetID)
		if err := cockpit.SwitchAccount(targetID); err != nil {
			log.Printf("[Cockpit] SwitchAccount failed: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		log.Printf("[Cockpit] Account switched successfully to: %s", targetID)
		writeJSON(w, http.StatusOK, map[string]any{
			"status":     "ok",
			"message":    "account switched successfully",
			"account_id": targetID,
		})
	})

	// Proxy routes: APIs, WebSocket, Artifacts, Gateway status
	rootMux.Handle("/api/", p)
	rootMux.Handle("/gateway/", p)
	rootMux.Handle("/static/artifacts/", p)
	rootMux.Handle("/connect-websocket", p)

	// Web frontend (catch-all)
	rootMux.Handle("/", webHandler)

	// 7. Wrap with AuthMiddleware
	router := auth.AuthMiddleware(authStore, rootMux)

	server := &http.Server{
		Addr:        fmt.Sprintf("%s:%d", *host, *port),
		Handler:     router,
		ReadTimeout: 60 * time.Second,
		// WriteTimeout is intentionally 0 (disabled) to avoid cutting off
		// WebSocket and SSE long-lived connections. Each handler manages
		// its own response timeouts via context.WithTimeout.
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	// Print initial status
	if cur := insp.Current(); cur != nil && cur.IsHealthy {
		log.Printf("✅ Upstream connected: 127.0.0.1:%d (PID %d)", cur.Port, cur.PID)
	} else {
		log.Printf("⚠️  Upstream Antigravity instance not detected yet, waiting...")
	}

	// Graceful shutdown channel
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		var err error
		if *tlsCert != "" && *tlsKey != "" {
			log.Printf("🔒 TLS enabled with cert=%s key=%s", *tlsCert, *tlsKey)
			err = server.ListenAndServeTLS(*tlsCert, *tlsKey)
		} else {
			err = server.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	if qrHost != "127.0.0.1" {
		log.Printf("📱 Mobile Web UI ready at: http://%s:%d (LAN) | http://127.0.0.1:%d (Local)", qrHost, *port, *port)
	} else {
		log.Printf("📱 Mobile Web UI ready at: http://127.0.0.1:%d", *port)
	}

	<-stopCh
	log.Println("Shutting down gateway...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
	log.Println("Gateway stopped gracefully.")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("[HTTP] Failed to encode JSON response: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(status)
	w.Write(data)
}
