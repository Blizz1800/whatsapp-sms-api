package models

type MessageContent struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	Caption string `json:"caption,omitempty"`
}

type MessageResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message,omitempty"`
	Data    *MessageContent `json:"data,omitempty"`
}

type ProtectMessageRequest struct {
	MessageID string `json:"message_id"`
}
