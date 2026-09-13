package cockpit

import (
	"context"
	"log"
	"time"
)

// StartQuotaAutoRefresher starts a background goroutine that checks whether Cockpit quota cache is stale
// and triggers Cockpit quota refresh dynamically based on actual data age.
func StartQuotaAutoRefresher(ctx context.Context, defaultInterval time.Duration) {
	if defaultInterval <= 0 {
		defaultInterval = 10 * time.Minute
	}

	go func() {
		configuredInterval := GetAutoRefreshInterval(defaultInterval)
		log.Printf("[Cockpit] Auto refresher started (target interval: %v, heartbeat: 30s)", configuredInterval)

		var lastTriggered time.Time

		checkAndRefresh := func() {
			targetInterval := GetAutoRefreshInterval(defaultInterval)
			quotas, err := GetQuotas()

			var lastUpdated time.Time
			if err == nil && quotas != nil && quotas.UpdatedAt > 0 {
				lastUpdated = time.UnixMilli(quotas.UpdatedAt)
			}

			now := time.Now()
			isExpired := lastUpdated.IsZero() || now.Sub(lastUpdated) >= targetInterval
			isRecentlyTriggered := !lastTriggered.IsZero() && now.Sub(lastTriggered) < 2*time.Minute

			if isExpired && !isRecentlyTriggered {
				if lastUpdated.IsZero() {
					log.Printf("[Cockpit] Quota data missing or uninitialized, triggering refresh...")
				} else {
					log.Printf("[Cockpit] Quota data is %v old (target: %v), triggering auto refresh...",
						now.Sub(lastUpdated).Round(time.Second), targetInterval)
				}
				lastTriggered = now
				if err := TriggerRefresh(); err != nil {
					log.Printf("[Cockpit] TriggerRefresh error: %v", err)
				}
			}
		}

		// Initial check on startup
		checkAndRefresh()

		// Heartbeat check every 30 seconds
		heartbeatTicker := time.NewTicker(30 * time.Second)
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
