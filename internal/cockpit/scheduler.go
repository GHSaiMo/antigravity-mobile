package cockpit

import (
	"context"
	"log"
	"time"
)

// StartQuotaAutoRefresher starts a background goroutine that periodically triggers Cockpit quota refresh.
func StartQuotaAutoRefresher(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Minute
	}

	go func() {
		log.Printf("[Cockpit] Auto refresher started with interval: %v", interval)

		// On startup: check if current quotas are older than the interval or nonexistent
		quotas, err := GetQuotas()
		needInitialRefresh := false
		if err != nil || quotas == nil {
			needInitialRefresh = true
		} else {
			lastUpdated := time.UnixMilli(quotas.UpdatedAt)
			if time.Since(lastUpdated) >= interval {
				needInitialRefresh = true
			}
		}

		if needInitialRefresh {
			log.Printf("[Cockpit] Initial quota check shows data is older than %v or missing, triggering startup refresh...", interval)
			if err := TriggerRefresh(); err != nil {
				log.Printf("[Cockpit] Startup TriggerRefresh error: %v", err)
			}
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Println("[Cockpit] Auto refresher stopped")
				return
			case <-ticker.C:
				log.Printf("[Cockpit] Scheduled auto refresh triggered (interval: %v)...", interval)
				if err := TriggerRefresh(); err != nil {
					log.Printf("[Cockpit] Scheduled TriggerRefresh error: %v", err)
				}
			}
		}
	}()
}
