package whatsapp

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"main/config"
)

func setupJobTestDB(t *testing.T) {
	t.Helper()
	t.Setenv("DB_TYPE", "sqlite")
	t.Setenv("DATABASE_URL", "file:"+filepath.ToSlash(filepath.Join(t.TempDir(), "test.db"))+"?_foreign_keys=on")
	var err error
	db, err = sql.Open("sqlite3", config.Load().DSN())
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if err := createTables(); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		db = nil
	})
}

func TestCreateJobDeduplicatesByKey(t *testing.T) {
	setupJobTestDB(t)
	ctx := context.Background()
	job := Job{
		ChatID:         "120363012345678901@g.us",
		Payload:        JobPayload{Type: "text", Text: "hola"},
		IdempotencyKey: "delivery-1",
	}
	first, dedup, err := CreateJob(ctx, job)
	if err != nil || dedup {
		t.Fatalf("first create: job=%v dedup=%v err=%v", first, dedup, err)
	}
	if first.Status != JobStatusQueued || first.Type != "text" {
		t.Fatalf("status/type = %q/%q, want queued/text", first.Status, first.Type)
	}
	if _, err := time.Parse(time.RFC3339, first.CreatedAt); err != nil {
		t.Fatalf("created_at %q is not RFC3339: %v", first.CreatedAt, err)
	}

	second, dedup, err := CreateJob(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	if !dedup || second.ID != first.ID {
		t.Fatalf("dedup=%v id=%q, want true/%q", dedup, second.ID, first.ID)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM message_jobs").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
}

func TestCreateJobWithoutKeyCreatesDistinctJobs(t *testing.T) {
	setupJobTestDB(t)
	ctx := context.Background()
	a, _, err := CreateJob(ctx, Job{ChatID: "a@s.whatsapp.net", Payload: JobPayload{Type: "text", Text: "a"}})
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := CreateJob(ctx, Job{ChatID: "a@s.whatsapp.net", Payload: JobPayload{Type: "text", Text: "a"}})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID {
		t.Fatalf("expected distinct IDs, got %q", a.ID)
	}
}

func TestGetJobNotFound(t *testing.T) {
	setupJobTestDB(t)
	_, err := GetJob(context.Background(), "missing")
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("err = %v, want ErrJobNotFound", err)
	}
}

func TestClaimNextJobOrdersByPriority(t *testing.T) {
	setupJobTestDB(t)
	ctx := context.Background()
	low, _, err := CreateJob(ctx, Job{ChatID: "low@s.whatsapp.net", Priority: 0, Payload: JobPayload{Type: "text", Text: "low"}})
	if err != nil {
		t.Fatal(err)
	}
	high, _, err := CreateJob(ctx, Job{ChatID: "high@s.whatsapp.net", Priority: 100, Payload: JobPayload{Type: "text", Text: "high"}})
	if err != nil {
		t.Fatal(err)
	}

	first, err := ClaimNextJob(ctx, nowTS())
	if err != nil || first == nil {
		t.Fatalf("first claim: job=%v err=%v", first, err)
	}
	if first.ID != high.ID || first.Status != JobStatusRunning {
		t.Fatalf("first = %q status=%q, want %q/running", first.ID, first.Status, high.ID)
	}
	second, err := ClaimNextJob(ctx, nowTS())
	if err != nil || second == nil || second.ID != low.ID {
		t.Fatalf("second claim: job=%v err=%v, want %q", second, err, low.ID)
	}
	third, err := ClaimNextJob(ctx, nowTS())
	if err != nil || third != nil {
		t.Fatalf("third claim: job=%v err=%v, want nil", third, err)
	}
}

func TestClaimNextJobRespectsNextAttemptAt(t *testing.T) {
	setupJobTestDB(t)
	ctx := context.Background()
	future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	if _, _, err := CreateJob(ctx, Job{ChatID: "a@s.whatsapp.net", NextAttemptAt: future, Payload: JobPayload{Type: "text", Text: "a"}}); err != nil {
		t.Fatal(err)
	}
	job, err := ClaimNextJob(ctx, nowTS())
	if err != nil || job != nil {
		t.Fatalf("job=%v err=%v, want nil", job, err)
	}
}

func TestUpdateJobPersistsSucceededState(t *testing.T) {
	setupJobTestDB(t)
	ctx := context.Background()
	job, _, err := CreateJob(ctx, Job{ChatID: "a@s.whatsapp.net", Payload: JobPayload{Type: "text", Text: "a"}})
	if err != nil {
		t.Fatal(err)
	}
	running, err := ClaimNextJob(ctx, nowTS())
	if err != nil || running == nil {
		t.Fatalf("claim: job=%v err=%v", running, err)
	}
	now := nowTS()
	err = UpdateJob(ctx, running.ID, JobUpdate{
		Status:     JobStatusSucceeded,
		Attempts:   1,
		StartedAt:  now,
		FinishedAt: now,
		Result:     &JobResult{MessageID: "MID", ChatID: "a@s.whatsapp.net", SentAt: now},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := GetJob(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != JobStatusSucceeded || got.Attempts != 1 || got.Result == nil || got.Result.MessageID != "MID" {
		t.Fatalf("got = %+v", got)
	}
}

func TestRequeueJobPreservesAttemptsAndClearsTimes(t *testing.T) {
	setupJobTestDB(t)
	ctx := context.Background()
	job, _, err := CreateJob(ctx, Job{
		ChatID:      "a@s.whatsapp.net",
		Status:      JobStatusFailed,
		Attempts:    2,
		StartedAt:   "2026-01-02T03:04:05Z",
		FinishedAt:  "2026-01-02T03:04:10Z",
		LastError:   "temporary failure",
		Payload:     JobPayload{Type: "text", Text: "a"},
		MaxAttempts: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	now := "2026-01-02T03:05:00Z"
	if err := RequeueJob(ctx, job.ID, now); err != nil {
		t.Fatal(err)
	}
	got, err := GetJob(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != JobStatusQueued || got.Attempts != 2 || got.NextAttemptAt != now {
		t.Fatalf("status/attempts/nextAttemptAt = %q/%d/%q", got.Status, got.Attempts, got.NextAttemptAt)
	}
	if got.StartedAt != "" || got.FinishedAt != "" {
		t.Fatalf("started/finished = %q/%q, want empty", got.StartedAt, got.FinishedAt)
	}
}
