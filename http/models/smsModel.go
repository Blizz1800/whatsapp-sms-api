package models

type SendMessageRequest struct {
	Phone   string `json:"phone"`
	Message string `json:"message"`
}

type SendMessageResponse struct {
	Ok      bool   `json:"ok"`
	Message string `json:"message"`
}
