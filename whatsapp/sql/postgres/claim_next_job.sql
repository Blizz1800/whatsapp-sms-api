UPDATE message_jobs
SET status = 'running', started_at = $1, updated_at = $1
WHERE id = (
    SELECT id FROM message_jobs
    WHERE status = 'queued'
      AND (next_attempt_at = '' OR next_attempt_at <= $1)
    ORDER BY priority DESC, created_at ASC
    LIMIT 1
)
RETURNING id, session_id, idempotency_key, chat_id, payload, priority, status,
          attempts, max_attempts, next_attempt_at, last_error, result, generation,
          created_at, updated_at, started_at, finished_at;
