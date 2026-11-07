SELECT status, COUNT(*) FROM message_jobs WHERE finished_at >= $1 GROUP BY status;
