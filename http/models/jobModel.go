package models

type JobMediaRequest struct {
	URL      string `json:"url"`
	MimeType string `json:"mime_type,omitempty"`
	Caption  string `json:"caption,omitempty"`
}

type CreateMessageRequest struct {
	ChatID               string           `json:"chat_id"`
	Type                 string           `json:"type"`
	Text                 string           `json:"text,omitempty"`
	Media                *JobMediaRequest `json:"media,omitempty"`
	ForwardFromMessageID string           `json:"forward_from_message_id,omitempty"`
	CaptionOverride      *string          `json:"caption_override,omitempty"`
	Priority             int              `json:"priority,omitempty"`
	IdempotencyKey       string           `json:"idempotency_key,omitempty"`
}
