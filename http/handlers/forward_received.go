package handlers

import (
	"encoding/json"
	"log"
	"main/http/models"
	"net/http"
	"time"
)

func ForwardReceivedHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		resp := models.ForwardResponse{Success: false, Message: "Method not allowed"}
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(resp)
		return
	}

	if !isClientReady() {
		resp := models.ForwardResponse{Success: false, Message: "WhatsApp client not connected yet. Scan the QR code first."}
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(resp)
		return
	}

	var req models.ForwardReceivedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resp := models.ForwardResponse{Success: false, Message: "Invalid JSON body"}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
		return
	}

	if req.MessageID == "" {
		resp := models.ForwardResponse{Success: false, Message: "message_id is required"}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
		return
	}

	if len(req.Recipients) == 0 {
		resp := models.ForwardResponse{Success: false, Message: "At least one recipient is required"}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
		return
	}

	start := time.Now()
	results := whatAppClient.ForwardReceivedMessage(req.MessageID, req.Recipients, req.CaptionOverride)

	allSuccess := true
	okCount := 0
	for _, res := range results {
		if !res.Success {
			allSuccess = false
		} else {
			okCount++
		}
	}
	log.Printf("forward_received: id=%s recipients=%d ok=%d fail=%d success=%v duration=%s",
		req.MessageID, len(req.Recipients), okCount, len(results)-okCount,
		allSuccess, time.Since(start))

	status := http.StatusOK
	msg := "Message forwarded successfully"
	if !allSuccess {
		status = http.StatusMultiStatus
		msg = "Some forwards failed"
	}

	resp := models.ForwardResponse{
		Success: allSuccess,
		Message: msg,
		Results: results,
	}
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}
