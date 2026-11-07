SELECT id, session_id, idempotency_key, chat_id, payload, priority, status,
       attempts, max_attempts, next_attempt_at, last_error, result, generation,
       created_at, updated_at, started_at, finished_at
FROM message_jobs
WHERE id = ?;
