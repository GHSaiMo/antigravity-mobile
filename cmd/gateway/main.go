package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

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

	// 3. Initialize Push Notification & Background Watcher
	notifCfg := config.GetNotificationConfig()
	watcherCtx, cancelWatcher := context.WithCancel(context.Background())
	defer cancelWatcher()

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

	// 4. Web frontend handler
	webHandler := web.Handler()

	// 5. Combined Root Router
	router := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Route to proxy for APIs, WebSockets, Artifacts, and Gateway status
		if strings.HasPrefix(path, "/api/") ||
			strings.HasPrefix(path, "/gateway/") ||
			strings.HasPrefix(path, "/static/artifacts/") ||
			path == "/connect-websocket" {
			p.ServeHTTP(w, r)
			return
		}

		// Route to web frontend for everything else
		webHandler.ServeHTTP(w, r)
	})

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", *host, *port),
		Handler:      router,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
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
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	log.Printf("📱 Mobile Web UI ready at: http://127.0.0.1:%d", *port)

	<-stopCh
	log.Println("Shutting down gateway...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
	log.Println("Gateway stopped gracefully.")
}
