package whatsapp

import (
	"os"
	"strconv"
	"time"
)

// StartCleanupLoop runs a background job that periodically deletes forwardable
// messages older than MSG_RETENTION_DAYS. This is a safety net for protected
// messages whose schedule was never released (e.g. after a bot crash).
func StartCleanupLoop() {
	if db == nil {
		return
	}

	interval := 24
	if v := os.Getenv("CLEANUP_INTERVAL_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			interval = n
		}
	}

	retentionDays := 30
	if v := os.Getenv("MSG_RETENTION_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			retentionDays = n
		}
	}

	ticker := time.NewTicker(time.Duration(interval) * time.Hour)
	go func() {
		for range ticker.C {
			DeleteOldForwardableMessages(retentionDays)
		}
	}()
}
