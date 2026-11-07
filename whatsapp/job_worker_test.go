package whatsapp

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDecideJobUpdate(t *testing.T) {
	cfg := WorkerConfig{MaxAttempts: 3, BackoffBase: 30 * time.Second, BackoffMax: 5 * time.Minute}
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	job := &Job{Attempts: 0, MaxAttempts: 3}

	ok := decideJobUpdate(job, JobResult{MessageID: "MID"}, nil, cfg, now)
	if ok.Status != JobStatusSucceeded || ok.Attempts != 1 || ok.FinishedAt == "" {
		t.Fatalf("success update = %+v", ok)
	}

	permanent := decideJobUpdate(job, JobResult{}, &JobError{Err: ErrPermanent}, cfg, now)
	if permanent.Status != JobStatusFailed || permanent.Attempts != 1 {
		t.Fatalf("permanent update = %+v", permanent)
	}

	retry := decideJobUpdate(job, JobResult{}, &JobError{Err: errors.New("timeout"), Retryable: true, Ambiguous: true}, cfg, now)
	if retry.Status != JobStatusQueued || retry.Attempts != 1 {
		t.Fatalf("retry update = %+v", retry)
	}
	if retry.NextAttemptAt != now.Add(30*time.Second).Format(time.RFC3339) {
		t.Fatalf("next = %q", retry.NextAttemptAt)
	}

	exhausted := &Job{Attempts: 2, MaxAttempts: 3}
	terminal := decideJobUpdate(exhausted, JobResult{}, &JobError{Err: errors.New("timeout"), Retryable: true, Ambiguous: true}, cfg, now)
	if terminal.Status != JobStatusUnknown || terminal.Attempts != 3 || terminal.FinishedAt == "" {
		t.Fatalf("terminal update = %+v", terminal)
	}
}

func TestBackoff(t *testing.T) {
	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{1, 30 * time.Second},
		{2, 60 * time.Second},
		{3, 120 * time.Second},
		{4, 240 * time.Second},
		{5, 300 * time.Second},
		{6, 300 * time.Second},
	}
	for _, c := range cases {
		if got := backoff(c.attempts, 30*time.Second, 5*time.Minute); got != c.want {
			t.Fatalf("backoff(%d) = %s, want %s", c.attempts, got, c.want)
		}
	}
}

type fakeSender struct {
	connected bool
	result    JobResult
	err       error
	calls     int
}

func (f *fakeSender) IsConnected() bool { return f.connected }
func (f *fakeSender) SendJob(ctx context.Context, job *Job) (JobResult, error) {
	f.calls++
	return f.result, f.err
}

type fakeGate struct {
	err    error
	cancel context.CancelFunc
}

func (f *fakeGate) Wait(ctx context.Context) error {
	if f.cancel != nil {
		f.cancel()
		return ctx.Err()
	}
	return f.err
}

func TestWorkerMarksSucceeded(t *testing.T) {
	setupJobTestDB(t)
	ctx := context.Background()
	job, _, err := CreateJob(ctx, Job{ChatID: "a@s.whatsapp.net", Payload: JobPayload{Type: "text", Text: "a"}})
	if err != nil {
		t.Fatal(err)
	}
	sender := &fakeSender{connected: true, result: JobResult{MessageID: "MID"}}
	worker := NewJobWorker(sender, &fakeGate{}, WorkerConfig{
		SendTimeout: time.Second, MaxAttempts: 3, BackoffBase: 30 * time.Second,
		BackoffMax: 5 * time.Minute, PollInterval: 10 * time.Millisecond,
		StaleAfter: time.Minute, SweepEvery: time.Minute,
	})
	n, err := worker.RunOnce(ctx)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	got, err := GetJob(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != JobStatusSucceeded {
		t.Fatalf("status = %q, want succeeded", got.Status)
	}
}

func TestWorkerDoesNotClaimWhenDisconnected(t *testing.T) {
	setupJobTestDB(t)
	ctx := context.Background()
	if _, _, err := CreateJob(ctx, Job{ChatID: "a@s.whatsapp.net", Payload: JobPayload{Type: "text", Text: "a"}}); err != nil {
		t.Fatal(err)
	}
	sender := &fakeSender{connected: false}
	worker := NewJobWorker(sender, &fakeGate{}, WorkerConfig{MaxAttempts: 3})
	n, err := worker.RunOnce(ctx)
	if err != nil || n != 0 || sender.calls != 0 {
		t.Fatalf("n=%d calls=%d err=%v", n, sender.calls, err)
	}
}

func TestWorkerRequeuesWhenGateFails(t *testing.T) {
	setupJobTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	job, _, err := CreateJob(ctx, Job{ChatID: "a@s.whatsapp.net", Payload: JobPayload{Type: "text", Text: "a"}})
	if err != nil {
		t.Fatal(err)
	}
	sender := &fakeSender{connected: true}
	worker := NewJobWorker(sender, &fakeGate{cancel: cancel}, WorkerConfig{MaxAttempts: 3})
	n, err := worker.RunOnce(ctx)
	if n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("n=%d err=%v, want 0/canceled", n, err)
	}
	got, err := GetJob(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != JobStatusQueued || got.Attempts != 0 || got.StartedAt != "" {
		t.Fatalf("status/attempts/startedAt = %q/%d/%q, want queued/0/empty", got.Status, got.Attempts, got.StartedAt)
	}
}
