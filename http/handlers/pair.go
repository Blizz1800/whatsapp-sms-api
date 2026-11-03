package handlers

import (
	"encoding/json"
	"errors"
	"main/http/models"
	"main/whatsapp"
	"net/http"
	"strings"
)

const pairSuccessMessage = "Enter this code in WhatsApp > Linked devices > Link with phone number. The code expires shortly."

func PairHandler(w http.ResponseWriter, r *http.Request) {
	pairHandler(defaultPairPhone).ServeHTTP(w, r)
}

func defaultPairPhone(phone string) (string, error) {
	if whatAppClient == nil {
		return "", whatsapp.ErrLoginNotReady
	}
	return whatAppClient.PairPhone(phone)
}

func pairHandler(pair func(phone string) (string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPost {
			resp := models.PairPhoneResponse{Success: false, Message: "Method not allowed"}
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(resp)
			return
		}

		var req models.PairPhoneRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			resp := models.PairPhoneResponse{Success: false, Message: "Invalid JSON body"}
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(resp)
			return
		}

		phone := strings.TrimSpace(req.Phone)
		if phone == "" {
			resp := models.PairPhoneResponse{Success: false, Message: "Phone is required"}
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(resp)
			return
		}

		code, err := pair(phone)
		if err != nil {
			status := http.StatusInternalServerError
			switch {
			case errors.Is(err, whatsapp.ErrAlreadyLinked):
				status = http.StatusConflict
			case errors.Is(err, whatsapp.ErrInvalidPhone):
				status = http.StatusBadRequest
			case errors.Is(err, whatsapp.ErrLoginNotReady):
				status = http.StatusServiceUnavailable
			}

			resp := models.PairPhoneResponse{Success: false, Message: err.Error()}
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(resp)
			return
		}

		resp := models.PairPhoneResponse{
			Success: true,
			Code:    code,
			Message: pairSuccessMessage,
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}
}
