package models

type PairPhoneRequest struct {
	Phone string `json:"phone"`
}

type PairPhoneResponse struct {
	Success bool   `json:"success"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}
