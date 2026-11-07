CREATE TABLE IF NOT EXISTS message_jobs (
    id              TEXT PRIMARY KEY,
    session_id      TEXT NOT NULL DEFAULT 'default',
    idempotency_key TEXT NOT NULL DEFAULT '',
    chat_id         TEXT NOT NULL,
    payload         TEXT NOT NULL,
    priority        INTEGER NOT NULL DEFAULT 0,
    status          TEXT NOT NULL DEFAULT 'queued',
    attempts        INTEGER NOT NULL DEFAULT 0,
    max_attempts    INTEGER NOT NULL DEFAULT 3,
    next_attempt_at TEXT NOT NULL DEFAULT '',
    last_error      TEXT NOT NULL DEFAULT '',
    result          TEXT NOT NULL DEFAULT '',
    generation      INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    started_at      TEXT NOT NULL DEFAULT '',
    finished_at     TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_message_jobs_idem
    ON message_jobs(session_id, idempotency_key)
    WHERE idempotency_key <> '';

CREATE INDEX IF NOT EXISTS idx_message_jobs_queue
    ON message_jobs(status, next_attempt_at, priority, created_at);
