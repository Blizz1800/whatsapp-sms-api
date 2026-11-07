INSERT INTO message_jobs (
    id, session_id, idempotency_key, chat_id, payload, priority, status,
    attempts, max_attempts, next_attempt_at, last_error, result, generation,
    created_at, updated_at, started_at, finished_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
ON CONFLICT (session_id, idempotency_key) WHERE idempotency_key <> '' DO NOTHING;
