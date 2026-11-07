UPDATE message_jobs
SET status='queued', attempts=0, next_attempt_at='', last_error='', result='',
    generation=generation+1, updated_at=$2, started_at='', finished_at=''
WHERE id=$1 AND status IN ('failed','unknown');
