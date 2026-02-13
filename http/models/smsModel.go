package models

type SendMessageRequest struct {
	Phone   string `json:"phone"`
	Message string `json:"message"`
}

type SendMessageResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}
