package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DBType                 string
	DatabaseURL            string
	CleanupIntervalHours   int
	MsgRetentionDays       int
	SendRatePerMinute      int
	SendTimeoutSeconds     int
	MaxSendAttempts        int
	RetryBackoffSeconds    int
	RetryBackoffMaxSeconds int
	JobPollIntervalMS      int
	JobStaleSeconds        int
	JobSweepInterval       time.Duration
	SyncWaitSeconds        int
	UpdatesBufferSize      int
}

const defaultConfigPath = "app.conf"

func Load() *Config {
	cfg := &Config{
		DBType:                 "postgres",
		CleanupIntervalHours:   24,
		MsgRetentionDays:       30,
		SendRatePerMinute:      20,
		SendTimeoutSeconds:     15,
		MaxSendAttempts:        3,
		RetryBackoffSeconds:    30,
		RetryBackoffMaxSeconds: 300,
		JobPollIntervalMS:      500,
		JobStaleSeconds:        300,
		JobSweepInterval:       60 * time.Second,
		SyncWaitSeconds:        30,
		UpdatesBufferSize:      1000,
	}
	parseFile(cfg, defaultConfigPath)
	applyEnvOverrides(cfg)
	return cfg
}

func parseFile(cfg *Config, path string) {
	f, err := os.Open(path)
	if err != nil {
		fmt.Printf("Warning: %s not found, using env/defaults\n", path)
		return
	}
	defer f.Close()

	section := ""
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 || section != "database" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(parts[0])) {
		case "type":
			cfg.DBType = strings.ToLower(strings.TrimSpace(parts[1]))
		case "url":
			cfg.DatabaseURL = strings.TrimSpace(parts[1])
		}
	}
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("DB_TYPE"); v != "" {
		cfg.DBType = strings.ToLower(v)
	}
	if v := os.Getenv("DATABASE_URL"); v != "" {
		cfg.DatabaseURL = v
	}
	cfg.SendRatePerMinute = envInt("SEND_RATE_PER_MINUTE", cfg.SendRatePerMinute, 1)
	cfg.SendTimeoutSeconds = envInt("SEND_TIMEOUT_SECONDS", cfg.SendTimeoutSeconds, 1)
	cfg.MaxSendAttempts = envInt("MAX_SEND_ATTEMPTS", cfg.MaxSendAttempts, 1)
	cfg.RetryBackoffSeconds = envInt("RETRY_BACKOFF_SECONDS", cfg.RetryBackoffSeconds, 0)
	cfg.RetryBackoffMaxSeconds = envInt("RETRY_BACKOFF_MAX_SECONDS", cfg.RetryBackoffMaxSeconds, 0)
	cfg.JobPollIntervalMS = envInt("JOB_POLL_INTERVAL_MS", cfg.JobPollIntervalMS, 0)
	cfg.JobStaleSeconds = envInt("JOB_STALE_SECONDS", cfg.JobStaleSeconds, 0)
	cfg.JobSweepInterval = envDuration("JOB_SWEEP_INTERVAL", cfg.JobSweepInterval)
	cfg.SyncWaitSeconds = envInt("SYNC_WAIT_SECONDS", cfg.SyncWaitSeconds, 1)
	cfg.UpdatesBufferSize = envInt("UPDATES_BUFFER_SIZE", cfg.UpdatesBufferSize, 100)
}

func envInt(key string, def, min int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < min {
		return def
	}
	return n
}

func envDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return def
	}
	return d
}

func (c *Config) Driver() string {
	if c.DBType == "sqlite" {
		return "sqlite3" // registrado por modernc en db.go
	}
	return "pgx"
}

func (c *Config) DSN() string {
	if c.DatabaseURL == "" && c.DBType == "sqlite" {
		return "file:///whatsapp.db"
	}
	return c.DatabaseURL
}
