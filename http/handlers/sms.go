package handlers

import (
	"encoding/json"
	"main/http/models"
	"net/http"
)

func SmsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		resp := models.SendMessageResponse{Ok: false, Message: "Method not allowed"}
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(resp)
		return
	}

	if whatAppClient == nil {
		resp := models.SendMessageResponse{Ok: false, Message: "WhatsApp client not initialized"}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(resp)
		return
	}

	var req models.SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resp := models.SendMessageResponse{Ok: false, Message: "Invalid JSON body"}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
		return
	}
	defer r.Body.Close()

	if req.Phone == "" || req.Message == "" {
		resp := models.SendMessageResponse{Ok: false, Message: "Phone and message are required"}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
		return
	}

	err := whatAppClient.SendMessage(req.Phone, req.Message)
	if err != nil {
		resp := models.SendMessageResponse{Ok: false, Message: err.Error()}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(resp)
		return
	}

	resp := models.SendMessageResponse{Ok: true, Message: "Message sent successfully"}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}
