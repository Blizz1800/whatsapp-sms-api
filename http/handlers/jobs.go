package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"main/config"
	"main/http/models"
	"main/whatsapp"
)

const (
	maxJobRequestBytes  = 1 << 20
	maxMessageTextBytes = 64 << 10
	maxJobIDs           = 100
)

type jobErrorResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

type createJobResponse struct {
	Success      bool   `json:"success"`
	JobID        string `json:"job_id"`
	Status       string `json:"status"`
	Deduplicated bool   `json:"deduplicated"`
}

func writeJobJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode job response: %v", err)
	}
}

func writeJobError(w http.ResponseWriter, status int, message string) {
	writeJobJSON(w, status, jobErrorResponse{Success: false, Message: message})
}

func decodeJobRequest(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJobRequestBytes)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(dst); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeJobError(w, http.StatusRequestEntityTooLarge, "Request body exceeds 1 MiB")
			return false
		}
		writeJobError(w, http.StatusBadRequest, "Invalid JSON body")
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeJobError(w, http.StatusBadRequest, "Request body must contain one JSON value")
		return false
	}
	return true
}

func validateCreateMessage(req models.CreateMessageRequest) error {
	if err := whatsapp.ValidateChatID(req.ChatID); err != nil {
		return fmt.Errorf("invalid chat_id: %w", err)
	}
	if req.Priority < 0 || req.Priority > 100 {
		return errors.New("priority must be between 0 and 100")
	}
	if len(req.IdempotencyKey) > 128 {
		return errors.New("idempotency_key must be at most 128 bytes")
	}
	switch req.Type {
	case "text":
		if req.Text == "" {
			return errors.New("text is required for type text")
		}
		if len(req.Text) > maxMessageTextBytes {
			return errors.New("text must be at most 64 KiB")
		}
	case "image":
		if req.Media == nil || req.Media.URL == "" {
			return errors.New("media.url is required for type image")
		}
		if len(req.Media.URL) > 2048 {
			return errors.New("media.url must be at most 2048 bytes")
		}
		parsed, err := url.ParseRequestURI(req.Media.URL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return errors.New("media.url must be an http or https URL")
		}
	case "forward":
		if req.ForwardFromMessageID == "" {
			return errors.New("forward_from_message_id is required for type forward")
		}
		if whatAppClient == nil || whatAppClient.GetMessageContent(req.ForwardFromMessageID) == nil {
			return errors.New("forward source message was not found")
		}
	default:
		return errors.New("type must be text, image, or forward")
	}
	return nil
}

func CreateMessageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJobError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	var req models.CreateMessageRequest
	if !decodeJobRequest(w, r, &req) {
		return
	}
	if err := validateCreateMessage(req); err != nil {
		writeJobError(w, http.StatusBadRequest, err.Error())
		return
	}
	payload := whatsapp.JobPayload{
		Type:                 req.Type,
		Text:                 req.Text,
		ForwardFromMessageID: req.ForwardFromMessageID,
		CaptionOverride:      req.CaptionOverride,
	}
	if req.Media != nil {
		payload.Media = &whatsapp.JobMedia{
			URL:      req.Media.URL,
			MimeType: req.Media.MimeType,
			Caption:  req.Media.Caption,
		}
	}
	job, deduplicated, err := whatsapp.CreateJob(r.Context(), whatsapp.Job{
		ChatID:         req.ChatID,
		Payload:        payload,
		Priority:       req.Priority,
		IdempotencyKey: req.IdempotencyKey,
		MaxAttempts:    config.Load().MaxSendAttempts,
	})
	if err != nil {
		log.Printf("create message job: %v", err)
		writeJobError(w, http.StatusInternalServerError, "Could not create message job")
		return
	}
	writeJobJSON(w, http.StatusAccepted, createJobResponse{
		Success:      true,
		JobID:        job.ID,
		Status:       job.Status,
		Deduplicated: deduplicated,
	})
}

func GetJobHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJobError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	job, err := whatsapp.GetJob(r.Context(), r.PathValue("id"))
	if errors.Is(err, whatsapp.ErrJobNotFound) {
		writeJobError(w, http.StatusNotFound, "Job not found")
		return
	}
	if err != nil {
		log.Printf("get message job: %v", err)
		writeJobError(w, http.StatusInternalServerError, "Could not retrieve message job")
		return
	}
	writeJobJSON(w, http.StatusOK, map[string]any{"success": true, "job": job})
}

func ListJobsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJobError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	raw := r.URL.Query().Get("ids")
	if raw == "" {
		writeJobError(w, http.StatusBadRequest, "ids query parameter is required")
		return
	}
	ids := strings.Split(raw, ",")
	if len(ids) > maxJobIDs {
		writeJobError(w, http.StatusBadRequest, "At most 100 job IDs may be requested")
		return
	}
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			writeJobError(w, http.StatusBadRequest, "Job IDs must not be empty")
			return
		}
	}
	jobs, err := whatsapp.GetJobs(r.Context(), ids)
	if err != nil {
		log.Printf("list message jobs: %v", err)
		writeJobError(w, http.StatusInternalServerError, "Could not retrieve message jobs")
		return
	}
	writeJobJSON(w, http.StatusOK, map[string]any{"success": true, "jobs": jobs})
}

func RetryJobHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJobError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	job, retried, err := whatsapp.RetryJob(r.Context(), r.PathValue("id"), time.Now().UTC().Format(time.RFC3339))
	if errors.Is(err, whatsapp.ErrJobNotFound) {
		writeJobError(w, http.StatusNotFound, "Job not found")
		return
	}
	if err != nil {
		log.Printf("retry message job: %v", err)
		writeJobError(w, http.StatusInternalServerError, "Could not retry message job")
		return
	}
	writeJobJSON(w, http.StatusOK, map[string]any{
		"success": true, "job_id": job.ID, "status": job.Status, "retried": retried,
	})
}

// waitForJob polls getJob every 100 ms until the job reaches a terminal state
// or timeout elapses. Returns the last observed job and whether it timed out.
func waitForJob(ctx context.Context, id string, timeout time.Duration, get func(string) (*whatsapp.Job, error), interval time.Duration) (*whatsapp.Job, bool) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		job, err := get(id)
		if err != nil {
			return nil, true
		}
		switch job.Status {
		case whatsapp.JobStatusSucceeded, whatsapp.JobStatusFailed, whatsapp.JobStatusUnknown:
			return job, false
		}
		select {
		case <-ctx.Done():
			return job, true
		case <-timer.C:
			return job, true
		case <-ticker.C:
		}
	}
}

func SendMessageSyncHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJobError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if !isClientReady() {
		writeJobError(w, http.StatusServiceUnavailable, "WhatsApp not connected")
		return
	}
	var req struct {
		ChatID string `json:"chat_id"`
		Text   string `json:"text"`
	}
	if !decodeJobRequest(w, r, &req) {
		return
	}
	if err := whatsapp.ValidateChatID(req.ChatID); err != nil {
		writeJobError(w, http.StatusBadRequest, fmt.Sprintf("invalid chat_id: %v", err))
		return
	}
	if req.Text == "" || len(req.Text) > maxMessageTextBytes {
		writeJobError(w, http.StatusBadRequest, "text is required and must be at most 64 KiB")
		return
	}
	job, _, err := whatsapp.CreateJob(r.Context(), whatsapp.Job{
		ChatID:         req.ChatID,
		Payload:        whatsapp.JobPayload{Type: "text", Text: req.Text},
		Priority:       100,
		MaxAttempts:    config.Load().MaxSendAttempts,
	})
	if err != nil {
		log.Printf("create message job: %v", err)
		writeJobError(w, http.StatusInternalServerError, "Could not create message job")
		return
	}
	cfg := config.Load()
	got, timedOut := waitForJob(r.Context(), job.ID, time.Duration(cfg.SyncWaitSeconds)*time.Second,
		func(id string) (*whatsapp.Job, error) { return whatsapp.GetJob(r.Context(), id) },
		100*time.Millisecond)
	if timedOut {
		writeJobJSON(w, http.StatusAccepted, map[string]any{
			"success": true, "job_id": job.ID, "status": "queued",
		})
		return
	}
	switch got.Status {
	case whatsapp.JobStatusSucceeded:
		writeJobJSON(w, http.StatusOK, map[string]any{
			"success": true, "message_id": got.Result.MessageID, "chat_id": got.ChatID,
		})
	case whatsapp.JobStatusFailed, whatsapp.JobStatusUnknown:
		writeJobJSON(w, http.StatusBadGateway, map[string]any{
			"success": false, "job_id": got.ID, "status": got.Status,
			"error": got.LastError,
		})
	default:
		writeJobJSON(w, http.StatusAccepted, map[string]any{
			"success": true, "job_id": got.ID, "status": got.Status,
		})
	}
}

func GetMeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJobError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if !isClientReady() {
		writeJobError(w, http.StatusServiceUnavailable, "WhatsApp not connected")
		return
	}
	id, phone, name, ok := whatAppClient.Identity()
	if !ok {
		writeJobError(w, http.StatusServiceUnavailable, "WhatsApp identity not available")
		return
	}
	writeJobJSON(w, http.StatusOK, map[string]any{
		"success": true, "id": id, "phone": phone, "name": name, "session_id": "default",
	})
}

func GetChatsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJobError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if !isClientReady() {
		writeJobError(w, http.StatusServiceUnavailable, "WhatsApp not connected")
		return
	}
	groups, err := whatAppClient.GetGroupsAndNewsletters()
	if err != nil {
		log.Printf("get groups and newsletters: %v", err)
		writeJobError(w, http.StatusInternalServerError, "Could not retrieve chats")
		return
	}
	chats := make([]map[string]any, 0, len(groups))
	for _, g := range groups {
		chatType := g.Type
		if chatType == "channel" {
			chatType = "newsletter"
		}
		chats = append(chats, map[string]any{"id": g.JID, "name": g.Name, "type": chatType})
	}
	writeJobJSON(w, http.StatusOK, map[string]any{"success": true, "chats": chats})
}
