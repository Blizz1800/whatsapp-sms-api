package config

import (
	"testing"
	"time"
)

func TestLoadGatewayDefaults(t *testing.T) {
	for _, k := range []string{
		"SEND_RATE_PER_MINUTE", "SEND_TIMEOUT_SECONDS", "MAX_SEND_ATTEMPTS",
		"RETRY_BACKOFF_SECONDS", "RETRY_BACKOFF_MAX_SECONDS", "JOB_POLL_INTERVAL_MS",
		"JOB_STALE_SECONDS", "JOB_SWEEP_INTERVAL", "SYNC_WAIT_SECONDS", "UPDATES_BUFFER_SIZE",
	} {
		t.Setenv(k, "")
	}
	cfg := Load()
	if cfg.SendRatePerMinute != 20 {
		t.Fatalf("SendRatePerMinute = %d, want 20", cfg.SendRatePerMinute)
	}
	if cfg.SendTimeoutSeconds != 15 {
		t.Fatalf("SendTimeoutSeconds = %d, want 15", cfg.SendTimeoutSeconds)
	}
	if cfg.MaxSendAttempts != 3 {
		t.Fatalf("MaxSendAttempts = %d, want 3", cfg.MaxSendAttempts)
	}
	if cfg.RetryBackoffSeconds != 30 || cfg.RetryBackoffMaxSeconds != 300 {
		t.Fatalf("backoff = %d/%d, want 30/300", cfg.RetryBackoffSeconds, cfg.RetryBackoffMaxSeconds)
	}
	if cfg.JobPollIntervalMS != 500 || cfg.JobStaleSeconds != 300 {
		t.Fatalf("poll/stale = %d/%d, want 500/300", cfg.JobPollIntervalMS, cfg.JobStaleSeconds)
	}
	if cfg.JobSweepInterval != 60*time.Second {
		t.Fatalf("JobSweepInterval = %s, want 60s", cfg.JobSweepInterval)
	}
	if cfg.SyncWaitSeconds != 30 || cfg.UpdatesBufferSize != 1000 {
		t.Fatalf("sync/buffer = %d/%d, want 30/1000", cfg.SyncWaitSeconds, cfg.UpdatesBufferSize)
	}
}

func TestLoadGatewayEnvOverrides(t *testing.T) {
	t.Setenv("SEND_RATE_PER_MINUTE", "10")
	t.Setenv("SEND_TIMEOUT_SECONDS", "20")
	t.Setenv("MAX_SEND_ATTEMPTS", "5")
	t.Setenv("JOB_SWEEP_INTERVAL", "15s")
	t.Setenv("UPDATES_BUFFER_SIZE", "2500")

	cfg := Load()
	if cfg.SendRatePerMinute != 10 || cfg.SendTimeoutSeconds != 20 {
		t.Fatalf("rate/timeout = %d/%d, want 10/20", cfg.SendRatePerMinute, cfg.SendTimeoutSeconds)
	}
	if cfg.MaxSendAttempts != 5 {
		t.Fatalf("MaxSendAttempts = %d, want 5", cfg.MaxSendAttempts)
	}
	if cfg.JobSweepInterval != 15*time.Second {
		t.Fatalf("JobSweepInterval = %s, want 15s", cfg.JobSweepInterval)
	}
	if cfg.UpdatesBufferSize != 2500 {
		t.Fatalf("UpdatesBufferSize = %d, want 2500", cfg.UpdatesBufferSize)
	}
}
