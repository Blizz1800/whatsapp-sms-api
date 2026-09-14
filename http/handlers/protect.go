package handlers

import (
	"encoding/json"
	"fmt"
	"main/http/models"
	"net/http"
)

func handleProtection(w http.ResponseWriter, r *http.Request, protect bool) {
	defer r.Body.Close()
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		resp := models.MessageResponse{Success: false, Message: "Method not allowed"}
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(resp)
		return
	}

	var req models.ProtectMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resp := models.MessageResponse{Success: false, Message: "Invalid JSON body"}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
		return
	}

	if req.MessageID == "" {
		resp := models.MessageResponse{Success: false, Message: "message_id is required"}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
		return
	}

	if protect {
		if !whatAppClient.ProtectMessage(req.MessageID) {
			resp := models.MessageResponse{Success: false, Message: "Message not found or not forwardable"}
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(resp)
			return
		}
	} else {
		whatAppClient.UnprotectMessage(req.MessageID)
	}

	action := "unprotected"
	if protect {
		action = "protected"
	}
	resp := models.MessageResponse{
		Success: true,
		Message: fmt.Sprintf("Message %s successfully", action),
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

func ProtectMessageHandler(w http.ResponseWriter, r *http.Request) {
	handleProtection(w, r, true)
}

func UnprotectMessageHandler(w http.ResponseWriter, r *http.Request) {
	handleProtection(w, r, false)
}
