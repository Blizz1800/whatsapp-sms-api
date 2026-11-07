SELECT status, COUNT(*) FROM message_jobs WHERE finished_at >= ? GROUP BY status;
