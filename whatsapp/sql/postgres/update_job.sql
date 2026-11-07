UPDATE message_jobs
SET status=$2, attempts=$3, next_attempt_at=$4, last_error=$5, result=$6,
    updated_at=$7, started_at=$8, finished_at=$9
WHERE id=$1;
