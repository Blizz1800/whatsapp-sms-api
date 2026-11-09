package handlers

import (
	"log"
	"net/http"
	"time"

	"main/config"
	"main/whatsapp"
)

func StatsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJobJSON(w, http.StatusMethodNotAllowed, jobErrorResponse{
			Success: false, Message: "Method not allowed",
		})
		return
	}
	since := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	all, sinceCounts, err := whatsapp.JobCounts(r.Context(), since)
	if err != nil {
		log.Printf("job counts: %v", err)
		writeJobJSON(w, http.StatusInternalServerError, jobErrorResponse{
			Success: false, Message: "Could not retrieve queue stats",
		})
		return
	}
	queue := map[string]any{
		"queued":      all[whatsapp.JobStatusQueued],
		"running":     all[whatsapp.JobStatusRunning],
		"failed_24h":  sinceCounts[whatsapp.JobStatusFailed],
		"unknown_24h": sinceCounts[whatsapp.JobStatusUnknown],
	}
	cfg := config.Load()
	writeJobJSON(w, http.StatusOK, map[string]any{
		"success":           true,
		"session_id":        "default",
		"client_ready":      isClientReady(),
		"queue":             queue,
		"rate_per_minute":   cfg.SendRatePerMinute,
		"send_timeout_seconds": cfg.SendTimeoutSeconds,
	})
}
