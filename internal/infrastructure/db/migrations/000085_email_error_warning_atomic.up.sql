-- Keep the most recent unresolved mailbox connection warning per account before
-- enforcing one active row. Preserve the latest occurrence timestamp.
WITH ranked AS (
  SELECT id, email_account_id,
         row_number() OVER (PARTITION BY email_account_id ORDER BY COALESCE(occurred_at, created_at) DESC, created_at DESC, id DESC) AS rn,
         max(COALESCE(occurred_at, created_at)) OVER (PARTITION BY email_account_id) AS latest
  FROM email_account_errors
  WHERE error_code = 'SERVER_UNREACHABLE' AND severity = 'WARNING'
    AND resolve_method = 'RETRY' AND task_id IS NULL AND resolved_at IS NULL
), refreshed AS (
  UPDATE email_account_errors e SET occurred_at = ranked.latest
  FROM ranked WHERE e.id = ranked.id AND ranked.rn = 1
  RETURNING e.id
)
UPDATE email_account_errors e
SET resolved_at = NOW(), resolved_by = 'system:DEDUP'
FROM ranked WHERE e.id = ranked.id AND ranked.rn > 1;

CREATE UNIQUE INDEX email_account_errors_one_active_connection_warning
ON email_account_errors (email_account_id)
WHERE error_code = 'SERVER_UNREACHABLE' AND severity = 'WARNING'
  AND resolve_method = 'RETRY' AND task_id IS NULL AND resolved_at IS NULL;
