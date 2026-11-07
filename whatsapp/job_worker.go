package whatsapp

import (
	"context"
	"errors"
	"log"
	"time"
)

type Gate interface {
	Wait(context.Context) error
}

type WorkerConfig struct {
	SendTimeout                          time.Duration
	MaxAttempts                          int
	BackoffBase, BackoffMax              time.Duration
	PollInterval, StaleAfter, SweepEvery time.Duration
}

type JobWorker struct {
	Sender JobSender
	Gate   Gate
	Config WorkerConfig
	Now    func() time.Time
}

func NewJobWorker(sender JobSender, gate Gate, cfg WorkerConfig) *JobWorker {
	if cfg.SendTimeout <= 0 {
		cfg.SendTimeout = 15 * time.Second
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.BackoffBase <= 0 {
		cfg.BackoffBase = 30 * time.Second
	}
	if cfg.BackoffMax <= 0 {
		cfg.BackoffMax = 5 * time.Minute
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 500 * time.Millisecond
	}
	if cfg.StaleAfter <= 0 {
		cfg.StaleAfter = 5 * time.Minute
	}
	if cfg.SweepEvery <= 0 {
		cfg.SweepEvery = time.Minute
	}
	return &JobWorker{Sender: sender, Gate: gate, Config: cfg, Now: time.Now}
}

func (w *JobWorker) Run(ctx context.Context) {
	now := w.now().UTC()
	recovered, err := RecoverRunningJobs(ctx, now.Format(time.RFC3339))
	if err != nil {
		log.Printf("job worker startup recovery failed: %v", err)
	} else if recovered > 0 {
		log.Printf("job worker recovered %d running job(s)", recovered)
	}

	pollTicker := time.NewTicker(w.Config.PollInterval)
	defer pollTicker.Stop()
	sweepTicker := time.NewTicker(w.Config.SweepEvery)
	defer sweepTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-pollTicker.C:
			if _, err := w.RunOnce(ctx); err != nil {
				log.Printf("job worker poll failed: %v", err)
			}
		case <-sweepTicker.C:
			now := w.now().UTC()
			cutoff := now.Add(-w.Config.StaleAfter).Format(time.RFC3339)
			swept, err := SweepStaleRunningJobs(ctx, now.Format(time.RFC3339), cutoff)
			if err != nil {
				log.Printf("job worker stale sweep failed: %v", err)
			} else if swept > 0 {
				log.Printf("job worker swept %d stale running job(s)", swept)
			}
		}
	}
}

func (w *JobWorker) RunOnce(ctx context.Context) (int, error) {
	now := w.now().UTC()
	if !w.Sender.IsConnected() {
		return 0, nil
	}
	job, err := ClaimNextJob(ctx, now.Format(time.RFC3339))
	if err != nil || job == nil {
		return 0, err
	}
	if err := w.Gate.Wait(ctx); err != nil {
		updateErr := UpdateJob(context.WithoutCancel(ctx), job.ID, JobUpdate{
			Status:        JobStatusQueued,
			Attempts:      job.Attempts,
			NextAttemptAt: now.Format(time.RFC3339),
			StartedAt:     "",
		})
		if updateErr != nil {
			return 0, errors.Join(err, updateErr)
		}
		return 0, err
	}

	sendCtx, cancel := context.WithTimeout(ctx, w.Config.SendTimeout)
	result, sendErr := w.Sender.SendJob(sendCtx, job)
	cancel()
	update := decideJobUpdate(job, result, sendErr, w.Config, w.now().UTC())
	if err := UpdateJob(context.WithoutCancel(ctx), job.ID, update); err != nil {
		return 1, err
	}
	return 1, nil
}

func decideJobUpdate(job *Job, result JobResult, sendErr error, cfg WorkerConfig, now time.Time) JobUpdate {
	attempts := job.Attempts + 1
	update := JobUpdate{
		Attempts:  attempts,
		StartedAt: job.StartedAt,
	}
	if sendErr == nil {
		update.Status = JobStatusSucceeded
		update.FinishedAt = now.UTC().Format(time.RFC3339)
		update.Result = &result
		return update
	}

	var jobErr *JobError
	if !errors.As(sendErr, &jobErr) {
		jobErr = classifySendError(sendErr)
	}
	result.Error = sendErr.Error()
	result.Retryable = jobErr.Retryable
	result.Ambiguous = jobErr.Ambiguous
	update.LastError = sendErr.Error()
	update.Result = &result
	if !jobErr.Retryable {
		update.Status = JobStatusFailed
		update.FinishedAt = now.UTC().Format(time.RFC3339)
		return update
	}

	maxAttempts := job.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = cfg.MaxAttempts
	}
	if attempts >= maxAttempts {
		update.Status = JobStatusFailed
		if jobErr.Ambiguous {
			update.Status = JobStatusUnknown
		}
		update.FinishedAt = now.UTC().Format(time.RFC3339)
		return update
	}
	update.Status = JobStatusQueued
	update.NextAttemptAt = now.UTC().Add(backoff(attempts, cfg.BackoffBase, cfg.BackoffMax)).Format(time.RFC3339)
	return update
}

func backoff(attempts int, base, max time.Duration) time.Duration {
	if base <= 0 || max <= 0 {
		return 0
	}
	if base >= max || attempts <= 1 {
		if base > max {
			return max
		}
		return base
	}
	delay := base
	for attempt := 1; attempt < attempts; attempt++ {
		if delay > max/2 {
			return max
		}
		delay *= 2
	}
	if delay > max {
		return max
	}
	return delay
}

func (w *JobWorker) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}
