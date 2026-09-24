package cockpit

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

var (
	schedulerMutex sync.Mutex
	isSelfHealing  bool
	backoffUntil   time.Time
	appStartupWait = 30 * time.Second
)

// ResetAutoRefreshCooldown clears any active failure backoff cooldown.
func ResetAutoRefreshCooldown() {
	schedulerMutex.Lock()
	backoffUntil = time.Time{}
	schedulerMutex.Unlock()
}

// StartQuotaAutoRefresher starts a background goroutine that checks whether Cockpit quota cache is stale
// and triggers Cockpit quota refresh dynamically based on actual data age.
// If refresh fails due to Cockpit Tools being down, it automatically attempts to launch the app,
// waits 30 seconds, probes again (up to 2 rounds), and sends an alert via alertFns if still unreachable.
func StartQuotaAutoRefresher(ctx context.Context, defaultInterval time.Duration, alertFns ...func(title, body string)) {
	if defaultInterval <= 0 {
		defaultInterval = 10 * time.Minute
	}

	var alertCallback func(title, body string)
	if len(alertFns) > 0 && alertFns[0] != nil {
		alertCallback = alertFns[0]
	}

	go func() {
		configuredInterval := GetAutoRefreshInterval(defaultInterval)
		log.Printf("[Cockpit] Auto refresher started (target interval: %v, heartbeat: 15s)", configuredInterval)

		checkAndRefresh := func() {
			schedulerMutex.Lock()
			if isSelfHealing {
				schedulerMutex.Unlock()
				return
			}
			if time.Now().Before(backoffUntil) {
				schedulerMutex.Unlock()
				return
			}
			schedulerMutex.Unlock()

			targetInterval := GetAutoRefreshInterval(defaultInterval)
			quotas, err := GetQuotas()

			var lastUpdated time.Time
			if err == nil && quotas != nil && quotas.UpdatedAt > 0 {
				lastUpdated = time.UnixMilli(quotas.UpdatedAt)
			}

			now := time.Now()
			isExpired := lastUpdated.IsZero() || now.Sub(lastUpdated) >= targetInterval
			if !isExpired {
				return
			}

			if lastUpdated.IsZero() {
				log.Printf("[Cockpit] Quota data missing or uninitialized, triggering refresh check...")
			} else {
				log.Printf("[Cockpit] Quota data is %v old (target: %v), triggering auto refresh check...",
					now.Sub(lastUpdated).Round(time.Second), targetInterval)
			}

			cfg, err := getCockpitConfig()
			if err != nil || cfg.ReportToken == "" || cfg.ReportToken == "change-this-token" || !cfg.ReportEnabled {
				isDefault := cfg != nil && cfg.ReportToken == "change-this-token"
				isEnabled := cfg != nil && cfg.ReportEnabled
				log.Printf("[Cockpit] ℹ️ Cockpit HTTP report service not configured or disabled (enabled=%v, defaultToken=%v). Run 'mgy cockpit' to configure.",
					isEnabled, isDefault)
				schedulerMutex.Lock()
				backoffUntil = time.Now().Add(targetInterval)
				schedulerMutex.Unlock()
				return
			}


			activePort, portErr := ResolveActiveReportPort(cfg.ReportToken, cfg.ReportPort)
			if portErr != nil && cfg.ReportPort > 0 {
				activePort = cfg.ReportPort
			}

			// First check reachability and execute report query
			var queryErr error
			if activePort <= 0 || !IsCockpitListening(activePort, 2*time.Second) {
				queryErr = fmt.Errorf("port %d not listening (connection refused)", activePort)
			} else {
				queryErr = QueryReport(activePort, cfg.ReportToken)
			}

			if queryErr == nil {
				schedulerMutex.Lock()
				backoffUntil = time.Time{}
				schedulerMutex.Unlock()
				return
			}

			// Enter self-healing mode
			schedulerMutex.Lock()
			isSelfHealing = true
			schedulerMutex.Unlock()

			defer func() {
				schedulerMutex.Lock()
				isSelfHealing = false
				schedulerMutex.Unlock()
			}()

			for round := 1; round <= 2; round++ {
				log.Printf("[Cockpit] ⚠️ Cockpit query error: %v. Attempting auto-launch Cockpit Tools app (Round %d/2)...", queryErr, round)

				if launchErr := LaunchCockpitApp(); launchErr != nil {
					log.Printf("[Cockpit] LaunchCockpitApp (Round %d/2) error: %v", round, launchErr)
				} else {
					log.Printf("[Cockpit] 🚀 Cockpit Tools app launch signal sent")
				}

				log.Printf("[Cockpit] Waiting %v for Cockpit Tools to initialize (Round %d/2)...", appStartupWait, round)
				select {
				case <-ctx.Done():
					log.Println("[Cockpit] Auto refresher cancelled during self-healing wait")
					return
				case <-time.After(appStartupWait):
				}

				// Re-resolve active report port after app launch in case port changed
				activePort, _ = ResolveActiveReportPort(cfg.ReportToken, cfg.ReportPort)
				if activePort <= 0 {
					activePort = cfg.ReportPort
				}

				log.Printf("[Cockpit] Probing Cockpit Tools port %d after Round %d launch...", activePort, round)
				if !IsCockpitListening(activePort, 3*time.Second) {
					queryErr = fmt.Errorf("port %d not listening", activePort)
				} else {
					queryErr = QueryReport(activePort, cfg.ReportToken)
				}

				if queryErr == nil {
					log.Printf("[Cockpit] ✅ Cockpit Tools recovered and quota refreshed successfully on Round %d!", round)
					schedulerMutex.Lock()
					backoffUntil = time.Time{}
					schedulerMutex.Unlock()
					return
				}
				log.Printf("[Cockpit] Round %d re-probe failed: %v", round, queryErr)
			}

			// Both rounds failed: notify via Bark and enter backoff cooldown
			log.Printf("[Cockpit] ❌ Cockpit Tools failed to recover after 2 launch rounds: %v", queryErr)
			if alertCallback != nil {
				alertCallback(
					"⚠️ 座舱助手未能启动",
					fmt.Sprintf("Cockpit Tools 自动拉起 2 轮后仍无法连接 (127.0.0.1:%d)，账号配额自动刷新已暂停。", activePort),
				)
			}

			schedulerMutex.Lock()
			backoffUntil = time.Now().Add(targetInterval)
			schedulerMutex.Unlock()
			log.Printf("[Cockpit] Entering %v cooldown before next auto-refresh attempt to prevent log flooding", targetInterval)
		}

		// Initial check on startup
		checkAndRefresh()

		// Heartbeat check every 15 seconds
		heartbeatTicker := time.NewTicker(15 * time.Second)
		defer heartbeatTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Println("[Cockpit] Auto refresher stopped")
				return
			case <-heartbeatTicker.C:
				checkAndRefresh()
			}
		}
	}()
}

