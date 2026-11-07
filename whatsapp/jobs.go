package whatsapp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"main/config"

	"github.com/google/uuid"
)

const (
	JobStatusQueued    = "queued"
	JobStatusRunning   = "running"
	JobStatusSucceeded = "succeeded"
	JobStatusFailed    = "failed"
	JobStatusUnknown   = "unknown"
)

var ErrJobNotFound = errors.New("job not found")

type JobPayload struct {
	Type                 string    `json:"type"`
	Text                 string    `json:"text,omitempty"`
	Media                *JobMedia `json:"media,omitempty"`
	ForwardFromMessageID string    `json:"forward_from_message_id,omitempty"`
	CaptionOverride      *string   `json:"caption_override,omitempty"`
}

type JobMedia struct {
	URL      string `json:"url"`
	MimeType string `json:"mime_type,omitempty"`
	Caption  string `json:"caption,omitempty"`
}

type JobResult struct {
	MessageID string `json:"message_id,omitempty"`
	ChatID    string `json:"chat_id,omitempty"`
	SentAt    string `json:"sent_at,omitempty"`
	Error     string `json:"error,omitempty"`
	Retryable bool   `json:"retryable"`
	Ambiguous bool   `json:"ambiguous"`
}

type Job struct {
	ID             string     `json:"id"`
	SessionID      string     `json:"session_id"`
	IdempotencyKey string     `json:"-"`
	ChatID         string     `json:"chat_id"`
	Type           string     `json:"type"`
	Payload        JobPayload `json:"-"`
	Result         *JobResult `json:"result,omitempty"`
	Priority       int        `json:"priority"`
	Status         string     `json:"status"`
	Attempts       int        `json:"attempts"`
	MaxAttempts    int        `json:"max_attempts"`
	Generation     int        `json:"-"`
	NextAttemptAt  string     `json:"next_attempt_at"`
	LastError      string     `json:"last_error"`
	CreatedAt      string     `json:"created_at"`
	UpdatedAt      string     `json:"updated_at"`
	StartedAt      string     `json:"started_at"`
	FinishedAt     string     `json:"finished_at"`
}

type rowScanner interface {
	Scan(dest ...any) error
}

func nowTS() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func scanJob(row rowScanner) (*Job, error) {
	job := &Job{}
	var payload, result string
	if err := row.Scan(
		&job.ID, &job.SessionID, &job.IdempotencyKey, &job.ChatID, &payload,
		&job.Priority, &job.Status, &job.Attempts, &job.MaxAttempts,
		&job.NextAttemptAt, &job.LastError, &result, &job.Generation,
		&job.CreatedAt, &job.UpdatedAt, &job.StartedAt, &job.FinishedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrJobNotFound
		}
		return nil, fmt.Errorf("scan message job: %w", err)
	}
	if err := json.Unmarshal([]byte(payload), &job.Payload); err != nil {
		return nil, fmt.Errorf("decode message job payload: %w", err)
	}
	job.Type = job.Payload.Type
	if result != "" {
		job.Result = &JobResult{}
		if err := json.Unmarshal([]byte(result), job.Result); err != nil {
			return nil, fmt.Errorf("decode message job result: %w", err)
		}
	}
	return job, nil
}

func CreateJob(ctx context.Context, job Job) (*Job, bool, error) {
	if db == nil {
		return nil, false, errors.New("database is not initialized")
	}
	if job.SessionID == "" {
		job.SessionID = "default"
	}
	if job.MaxAttempts <= 0 {
		job.MaxAttempts = 3
	}
	if job.Status == "" {
		job.Status = JobStatusQueued
	}
	if job.Type == "" {
		job.Type = job.Payload.Type
	}
	if job.ID == "" {
		job.ID = uuid.NewString()
	}
	if job.CreatedAt == "" {
		job.CreatedAt = nowTS()
	}
	if job.UpdatedAt == "" {
		job.UpdatedAt = job.CreatedAt
	}

	payload, err := json.Marshal(job.Payload)
	if err != nil {
		return nil, false, fmt.Errorf("marshal message job payload: %w", err)
	}
	var result string
	if job.Result != nil {
		encoded, err := json.Marshal(job.Result)
		if err != nil {
			return nil, false, fmt.Errorf("marshal message job result: %w", err)
		}
		result = string(encoded)
	}
	query, err := loadQuery("insert_job")
	if err != nil {
		return nil, false, fmt.Errorf("load insert job query: %w", err)
	}
	res, err := db.ExecContext(ctx, query,
		job.ID, job.SessionID, job.IdempotencyKey, job.ChatID, string(payload),
		job.Priority, job.Status, job.Attempts, job.MaxAttempts, job.NextAttemptAt,
		job.LastError, result, job.Generation, job.CreatedAt, job.UpdatedAt,
		job.StartedAt, job.FinishedAt,
	)
	if err != nil {
		return nil, false, fmt.Errorf("insert message job: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, false, fmt.Errorf("count inserted message job rows: %w", err)
	}
	if affected == 0 && job.IdempotencyKey != "" {
		existing, err := getJobByIdempotencyKey(ctx, job.SessionID, job.IdempotencyKey)
		if err != nil {
			return nil, false, err
		}
		return existing, true, nil
	}
	if affected == 0 {
		return nil, false, errors.New("message job insert affected no rows")
	}
	created, err := GetJob(ctx, job.ID)
	return created, false, err
}

func GetJob(ctx context.Context, id string) (*Job, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	query, err := loadQuery("get_job")
	if err != nil {
		return nil, fmt.Errorf("load get job query: %w", err)
	}
	job, err := scanJob(db.QueryRowContext(ctx, query, id))
	if err != nil {
		return nil, err
	}
	return job, nil
}

func getJobByIdempotencyKey(ctx context.Context, sessionID, idempotencyKey string) (*Job, error) {
	query, err := loadQuery("get_job_by_idempotency_key")
	if err != nil {
		return nil, fmt.Errorf("load get job by idempotency key query: %w", err)
	}
	job, err := scanJob(db.QueryRowContext(ctx, query, sessionID, idempotencyKey))
	if err != nil {
		return nil, fmt.Errorf("get existing message job: %w", err)
	}
	return job, nil
}

type JobUpdate struct {
	Status                   string
	Attempts                 int
	NextAttemptAt, LastError string
	StartedAt, FinishedAt    string
	Result                   *JobResult
}

func ClaimNextJob(ctx context.Context, now string) (*Job, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	query, err := loadQuery("claim_next_job")
	if err != nil {
		return nil, fmt.Errorf("load claim next job query: %w", err)
	}
	cfg := config.Load()
	var row rowScanner
	if cfg.DBType == "sqlite" {
		row = db.QueryRowContext(ctx, query, now, now, now)
	} else {
		row = db.QueryRowContext(ctx, query, now)
	}
	job, err := scanJob(row)
	if errors.Is(err, ErrJobNotFound) {
		return nil, nil
	}
	return job, err
}

func UpdateJob(ctx context.Context, id string, update JobUpdate) error {
	if db == nil {
		return errors.New("database is not initialized")
	}
	var result string
	if update.Result != nil {
		encoded, err := json.Marshal(update.Result)
		if err != nil {
			return fmt.Errorf("marshal message job result: %w", err)
		}
		result = string(encoded)
	}
	query, err := loadQuery("update_job")
	if err != nil {
		return fmt.Errorf("load update job query: %w", err)
	}
	updatedAt := nowTS()
	args := []any{
		id, update.Status, update.Attempts, update.NextAttemptAt, update.LastError,
		result, updatedAt, update.StartedAt, update.FinishedAt,
	}
	if config.Load().DBType == "sqlite" {
		args = []any{
			update.Status, update.Attempts, update.NextAttemptAt, update.LastError,
			result, updatedAt, update.StartedAt, update.FinishedAt, id,
		}
	}
	res, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update message job: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("count updated message job rows: %w", err)
	}
	if affected == 0 {
		return ErrJobNotFound
	}
	return nil
}

func RequeueJob(ctx context.Context, id, now string) error {
	job, err := GetJob(ctx, id)
	if err != nil {
		return err
	}
	return UpdateJob(ctx, id, JobUpdate{
		Status:        JobStatusQueued,
		Attempts:      job.Attempts,
		NextAttemptAt: now,
		LastError:     job.LastError,
		StartedAt:     "",
		FinishedAt:    "",
		Result:        job.Result,
	})
}
