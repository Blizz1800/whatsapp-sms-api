DELETE FROM forwardable_messages
WHERE protected = 0
  AND created_at < datetime('now', '-' || ? || ' days')
