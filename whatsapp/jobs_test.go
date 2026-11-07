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
