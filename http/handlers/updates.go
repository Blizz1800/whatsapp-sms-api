package handlers

import (
	"net/http"
	"strconv"
	"time"
)

func GetUpdatesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJobJSON(w, http.StatusMethodNotAllowed, jobErrorResponse{
			Success: false, Message: "Method not allowed",
		})
		return
	}

	offset := int64(0)
	if v := r.URL.Query().Get("offset"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			writeJobJSON(w, http.StatusBadRequest, jobErrorResponse{
				Success: false, Message: "offset must be a non-negative integer",
			})
			return
		}
		offset = n
	}

	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeJobJSON(w, http.StatusBadRequest, jobErrorResponse{
				Success: false, Message: "limit must be a positive integer",
			})
			return
		}
		if n > 500 {
			n = 500
		}
		limit = n
	}

	timeout := 0
	if v := r.URL.Query().Get("timeout"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeJobJSON(w, http.StatusBadRequest, jobErrorResponse{
				Success: false, Message: "timeout must be a non-negative integer",
			})
			return
		}
		if n > 30 {
			n = 30
		}
		timeout = n
	}

	updates := whatAppClient.GetUpdates(r.Context(), offset, limit, time.Duration(timeout)*time.Second)
	out := make([]any, 0, len(updates))
	for _, u := range updates {
		out = append(out, u)
	}
	writeJobJSON(w, http.StatusOK, map[string]any{"success": true, "updates": out, "count": len(updates)})
}
