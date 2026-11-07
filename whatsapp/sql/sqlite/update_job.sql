UPDATE message_jobs
SET status=?, attempts=?, next_attempt_at=?, last_error=?, result=?,
    updated_at=?, started_at=?, finished_at=?
WHERE id=?;
