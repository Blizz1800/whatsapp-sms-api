DELETE FROM forwardable_messages
WHERE protected = 0
  AND created_at < NOW() - ($1::int * INTERVAL '1 day')
